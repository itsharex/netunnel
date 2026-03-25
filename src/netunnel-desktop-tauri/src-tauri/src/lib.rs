use std::{
    env,
    fs::{self, File, OpenOptions},
    io::{Read, Write},
    net::{TcpStream, ToSocketAddrs},
    path::{Path, PathBuf},
    process::{Child, Command, ExitStatus},
    sync::Mutex,
    time::{Duration, Instant},
};

use chrono::Local;
use serde::{Deserialize, Serialize};
use sysinfo::{Pid, ProcessesToUpdate, Signal, System};
use tauri::{
    menu::{MenuBuilder, MenuItemBuilder},
    tray::TrayIconBuilder,
    Manager, Url,
};

#[cfg(desktop)]
use tauri_plugin_updater::UpdaterExt;

#[cfg(target_os = "windows")]
use std::os::windows::process::CommandExt;

#[derive(Default)]
struct PendingUpdate(Mutex<Option<tauri_plugin_updater::Update>>);

#[derive(Default)]
struct AgentRuntime(Mutex<AgentProcessState>);

struct AgentProcessState {
    child: Option<Child>,
    tracked_pid: Option<u32>,
    last_exit: Option<String>,
    last_process_probe: Option<AgentProcessProbeCache>,
}

#[derive(Clone, Copy)]
struct AgentProcessProbeCache {
    checked_at: Instant,
    tracked_pid: Option<u32>,
    tracked_alive: bool,
    detected_pid: Option<u32>,
}

impl Default for AgentProcessState {
    fn default() -> Self {
        Self {
            child: None,
            tracked_pid: None,
            last_exit: None,
            last_process_probe: None,
        }
    }
}

struct AppLogger {
    directory: PathBuf,
    file_path: PathBuf,
    file: Mutex<File>,
}

impl AppLogger {
    fn new(log_dir: PathBuf) -> Result<Self, String> {
        fs::create_dir_all(&log_dir).map_err(|error| error.to_string())?;
        let file_path = log_dir.join("app.log");
        let file = OpenOptions::new()
            .create(true)
            .append(true)
            .open(&file_path)
            .map_err(|error| error.to_string())?;

        Ok(Self {
            directory: log_dir,
            file_path,
            file: Mutex::new(file),
        })
    }

    fn write(&self, level: &str, message: impl AsRef<str>) {
        let timestamp = Local::now().format("%Y-%m-%d %H:%M:%S");
        let line = format!("[{timestamp}] [{level}] {}\n", message.as_ref());

        if let Ok(mut file) = self.file.lock() {
            let _ = file.write_all(line.as_bytes());
            let _ = file.flush();
        }
    }
}

#[cfg(target_os = "windows")]
const CREATE_NO_WINDOW: u32 = 0x08000000;

#[cfg(target_os = "windows")]
fn configure_background_command(command: &mut Command) {
    command.creation_flags(CREATE_NO_WINDOW);
}

#[cfg(not(target_os = "windows"))]
fn configure_background_command(_command: &mut Command) {}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct UpdaterStatus {
    enabled: bool,
    reason: Option<String>,
    current_version: String,
    endpoints: Vec<String>,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct UpdatePayload {
    version: String,
    current_version: String,
    date: Option<String>,
    body: Option<String>,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct LogStatus {
    directory: String,
    file_path: String,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct AgentStatus {
    running: bool,
    executable_path: String,
    arguments: Vec<String>,
    pid: Option<u32>,
    last_exit: Option<String>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct AgentLaunchInput {
    executable_path: Option<String>,
    arguments: Option<Vec<String>>,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct ProbeResult {
    ok: bool,
    address: String,
    message: String,
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct PersistedAgentRuntime {
    pid: Option<u32>,
}

fn open_in_file_manager(path: &Path) -> Result<(), String> {
    #[cfg(target_os = "windows")]
    {
        let mut command = Command::new("explorer");
        command.arg(path);
        configure_background_command(&mut command);
        command.spawn().map_err(|error| error.to_string())?;
    }

    #[cfg(target_os = "macos")]
    {
        Command::new("open")
            .arg(path)
            .spawn()
            .map_err(|error| error.to_string())?;
    }

    #[cfg(all(unix, not(target_os = "macos")))]
    {
        Command::new("xdg-open")
            .arg(path)
            .spawn()
            .map_err(|error| error.to_string())?;
    }

    Ok(())
}

fn updater_config() -> Result<serde_json::Value, String> {
    serde_json::from_str(include_str!("../tauri.conf.json"))
        .map_err(|error| format!("读取 tauri.conf.json 的 updater 配置失败: {}", error))
}

fn updater_public_key() -> Option<String> {
    let config = updater_config().ok()?;
    config
        .pointer("/plugins/updater/pubkey")
        .and_then(|value| value.as_str())
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_string)
}

fn updater_endpoints() -> Vec<String> {
    let Ok(config) = updater_config() else {
        return Vec::new();
    };

    config
        .pointer("/plugins/updater/endpoints")
        .and_then(|value| value.as_array())
        .map(|items| {
            items
                .iter()
                .filter_map(|item| item.as_str())
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(str::to_string)
                .collect()
        })
        .unwrap_or_default()
}

fn parsed_updater_endpoints() -> Result<Vec<Url>, String> {
    updater_endpoints()
        .into_iter()
        .map(|endpoint| Url::parse(&endpoint).map_err(|error| error.to_string()))
        .collect()
}

fn updater_disabled_reason() -> Option<String> {
    if let Err(error) = updater_config() {
        return Some(error);
    }

    if updater_public_key().is_none() {
        return Some("tauri.conf.json 未配置 updater pubkey，应用内更新已禁用。".into());
    }

    if updater_endpoints().is_empty() {
        return Some("tauri.conf.json 未配置 updater endpoints，应用内更新已禁用。".into());
    }

    None
}

fn agent_id_path(app: &tauri::AppHandle) -> Result<PathBuf, String> {
    let dir = app.path().app_data_dir().map_err(|e| e.to_string())?;
    Ok(dir.join("agent_id.json"))
}

fn agent_runtime_path(app: &tauri::AppHandle) -> Result<PathBuf, String> {
    let dir = app.path().app_data_dir().map_err(|e| e.to_string())?;
    Ok(dir.join("agent_runtime.json"))
}

#[tauri::command]
fn get_or_create_agent_id(app: tauri::AppHandle) -> Result<String, String> {
    let path = agent_id_path(&app)?;
    if path.exists() {
        let content = fs::read_to_string(&path).map_err(|e| e.to_string())?;
        let id: serde_json::Value = serde_json::from_str(&content).map_err(|e| e.to_string())?;
        id.get("agentId")
            .and_then(|v| v.as_str())
            .map(String::from)
            .ok_or_else(|| "agent_id.json 格式无效".to_string())
    } else {
        let id = uuid::Uuid::new_v4().to_string();
        let json = serde_json::json!({ "agentId": id });
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent).map_err(|e| e.to_string())?;
        }
        let content = serde_json::to_string_pretty(&json).map_err(|e| e.to_string())?;
        fs::write(&path, content).map_err(|e| e.to_string())?;
        Ok(id)
    }
}

#[tauri::command]
fn reset_agent_id(app: tauri::AppHandle) -> Result<String, String> {
    let path = agent_id_path(&app)?;
    let id = uuid::Uuid::new_v4().to_string();
    let json = serde_json::json!({ "agentId": id });
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).map_err(|e| e.to_string())?;
    }
    let content = serde_json::to_string_pretty(&json).map_err(|e| e.to_string())?;
    fs::write(&path, content).map_err(|e| e.to_string())?;
    Ok(id)
}

fn read_persisted_agent_pid(app: &tauri::AppHandle) -> Result<Option<u32>, String> {
    let path = agent_runtime_path(app)?;
    if !path.exists() {
        return Ok(None);
    }

    let content = fs::read_to_string(&path).map_err(|e| e.to_string())?;
    let runtime: PersistedAgentRuntime =
        serde_json::from_str(&content).map_err(|e| e.to_string())?;
    Ok(runtime.pid)
}

fn persist_agent_pid(app: &tauri::AppHandle, pid: Option<u32>) -> Result<(), String> {
    let path = agent_runtime_path(app)?;
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).map_err(|e| e.to_string())?;
    }

    let content = serde_json::to_string_pretty(&PersistedAgentRuntime { pid })
        .map_err(|e| e.to_string())?;
    fs::write(&path, content).map_err(|e| e.to_string())
}

fn clear_persisted_agent_pid(app: &tauri::AppHandle) -> Result<(), String> {
    let path = agent_runtime_path(app)?;
    if path.exists() {
        fs::remove_file(path).map_err(|e| e.to_string())?;
    }
    Ok(())
}

fn local_agent_executable(app: &tauri::AppHandle) -> Result<PathBuf, String> {
    let mut search_roots = Vec::new();

    if let Ok(dir) = app.path().executable_dir() {
        search_roots.push(dir);
    }

    if let Ok(exe) = env::current_exe() {
        if let Some(parent) = exe.parent() {
            search_roots.push(parent.to_path_buf());
        }
    }

    if let Ok(dir) = env::current_dir() {
        search_roots.push(dir);
    }

    if let Ok(resource_dir) = app.path().resource_dir() {
        search_roots.push(resource_dir);
    }

    for root in search_roots {
        let mut current = root;
        for _ in 0..8 {
            let direct_candidate = current.join("agent-run.exe");
            if direct_candidate.exists() {
                return Ok(direct_candidate);
            }

            let nested_candidate = current.join("netunnel-agent").join("agent-run.exe");
            if nested_candidate.exists() {
                return Ok(nested_candidate);
            }

            let src_candidate = current
                .join("src")
                .join("netunnel-agent")
                .join("agent-run.exe");
            if src_candidate.exists() {
                return Ok(src_candidate);
            }

            if !current.pop() {
                break;
            }
        }
    }

    let fallback = env::current_dir()
        .map(|dir| dir.join("src").join("netunnel-agent").join("agent-run.exe"))
        .unwrap_or_else(|_| PathBuf::from("src").join("netunnel-agent").join("agent-run.exe"));
    Ok(fallback)
}

fn resolve_agent_executable(app: &tauri::AppHandle, override_path: Option<String>) -> Result<PathBuf, String> {
    if let Some(path) = override_path {
        let trimmed = path.trim();
        if !trimmed.is_empty() {
            return Ok(PathBuf::from(trimmed));
        }
    }

    local_agent_executable(app)
}

fn show_main_window(app: &tauri::AppHandle) -> Result<(), String> {
    let window = app
        .get_webview_window("main")
        .ok_or_else(|| "找不到主窗口。".to_string())?;
    window.show().map_err(|error| error.to_string())?;
    window.set_focus().map_err(|error| error.to_string())?;
    Ok(())
}

fn hide_main_window(app: &tauri::AppHandle) -> Result<(), String> {
    let window = app
        .get_webview_window("main")
        .ok_or_else(|| "找不到主窗口。".to_string())?;
    window.hide().map_err(|error| error.to_string())?;
    Ok(())
}

#[tauri::command]
fn updater_status(app: tauri::AppHandle) -> UpdaterStatus {
    UpdaterStatus {
        enabled: updater_disabled_reason().is_none(),
        reason: updater_disabled_reason(),
        current_version: app.package_info().version.to_string(),
        endpoints: updater_endpoints(),
    }
}

#[tauri::command]
fn logger_status(logger: tauri::State<'_, AppLogger>) -> LogStatus {
    LogStatus {
        directory: logger.directory.display().to_string(),
        file_path: logger.file_path.display().to_string(),
    }
}

#[tauri::command]
fn open_logs_directory(logger: tauri::State<'_, AppLogger>) -> Result<(), String> {
    open_in_file_manager(&logger.directory)
}

#[tauri::command]
fn open_devtools(app: tauri::AppHandle) -> Result<(), String> {
    let window = app
        .get_webview_window("main")
        .ok_or_else(|| "找不到主窗口，无法打开调试控制台。".to_string())?;
    window.open_devtools();
    Ok(())
}

#[tauri::command]
fn hide_to_tray(app: tauri::AppHandle) -> Result<(), String> {
    hide_main_window(&app)
}

#[tauri::command]
fn show_main_window_command(app: tauri::AppHandle) -> Result<(), String> {
    show_main_window(&app)
}

fn format_exit_status(status: ExitStatus) -> String {
    match status.code() {
        Some(code) => format!("agent-run.exe exited with code {}", code),
        None => "agent-run.exe exited".to_string(),
    }
}

fn is_process_alive(pid: u32) -> Result<bool, String> {
    let mut system = System::new_all();
    system.refresh_processes(ProcessesToUpdate::All, true);
    Ok(system.process(Pid::from_u32(pid)).is_some())
}

fn probe_agent_processes(tracked_pid: Option<u32>) -> (bool, Option<u32>) {
    let mut system = System::new_all();
    system.refresh_processes(ProcessesToUpdate::All, true);

    let tracked_alive = tracked_pid
        .map(|pid| system.process(Pid::from_u32(pid)).is_some())
        .unwrap_or(false);

    let detected_pid = system.processes().iter().find_map(|(pid, process)| {
        let name = process.name().to_string_lossy().to_ascii_lowercase();
        if name == "agent-run.exe" || name == "agent-run" {
            Some(pid.as_u32())
        } else {
            None
        }
    });

    (tracked_alive, detected_pid)
}

fn kill_process(pid: u32) -> Result<(), String> {
    let mut system = System::new_all();
    system.refresh_processes(ProcessesToUpdate::All, true);

    if let Some(process) = system.process(Pid::from_u32(pid)) {
        if process.kill_with(Signal::Kill).unwrap_or(false) || process.kill() {
            return Ok(());
        }
        return Err(format!("结束进程失败，pid={}", pid));
    }

    Ok(())
}

fn sync_agent_process_state(
    app: &tauri::AppHandle,
    logger: &AppLogger,
    state: &mut AgentProcessState,
) -> Result<(bool, Option<u32>, Option<String>), String> {
    if let Some(child) = state.child.as_mut() {
        let pid = child.id();
        let wait_result = child.try_wait().map_err(|error| error.to_string())?;
        match wait_result {
            Some(status) => {
                let exit_message = format_exit_status(status);
                logger.write("WARN", exit_message.clone());
                state.child = None;
                state.tracked_pid = None;
                state.last_exit = Some(exit_message.clone());
                let _ = clear_persisted_agent_pid(app);
                Ok((false, None, Some(exit_message)))
            }
            None => {
                state.tracked_pid = Some(pid);
                let _ = persist_agent_pid(app, Some(pid));
                Ok((true, Some(pid), state.last_exit.clone()))
            }
        }
    } else {
        let tracked_pid = match state.tracked_pid {
            Some(pid) => Some(pid),
            None => read_persisted_agent_pid(app)?,
        };

        let cache_ttl = Duration::from_secs(3);
        let (tracked_alive, detected_pid) = if let Some(cache) = state.last_process_probe {
            if cache.tracked_pid == tracked_pid && cache.checked_at.elapsed() < cache_ttl {
                (cache.tracked_alive, cache.detected_pid)
            } else {
                let probe = probe_agent_processes(tracked_pid);
                state.last_process_probe = Some(AgentProcessProbeCache {
                    checked_at: Instant::now(),
                    tracked_pid,
                    tracked_alive: probe.0,
                    detected_pid: probe.1,
                });
                probe
            }
        } else {
            let probe = probe_agent_processes(tracked_pid);
            state.last_process_probe = Some(AgentProcessProbeCache {
                checked_at: Instant::now(),
                tracked_pid,
                tracked_alive: probe.0,
                detected_pid: probe.1,
            });
            probe
        };

        if let Some(pid) = tracked_pid {
            if tracked_alive {
                state.tracked_pid = Some(pid);
                return Ok((true, Some(pid), state.last_exit.clone()));
            }
            state.tracked_pid = None;
            let _ = clear_persisted_agent_pid(app);
        }

        if let Some(pid) = detected_pid {
            state.tracked_pid = Some(pid);
            let _ = persist_agent_pid(app, Some(pid));
            logger.write("INFO", format!("检测到已存在的本地 agent 进程，pid={}", pid));
            return Ok((true, Some(pid), state.last_exit.clone()));
        }

        Ok((false, None, state.last_exit.clone()))
    }
}

fn stop_agent_runtime_internal(
    app: &tauri::AppHandle,
    logger: &AppLogger,
    runtime: &AgentRuntime,
) -> Result<(), String> {
    let mut guard = runtime
        .0
        .lock()
        .map_err(|_| "无法锁定 agent 运行状态。".to_string())?;

    if let Some(mut child) = guard.child.take() {
        logger.write("INFO", "停止本地 agent");
        child.kill().map_err(|error| error.to_string())?;
        let _ = child.wait();
        guard.tracked_pid = None;
        guard.last_exit = Some("agent-run.exe stopped manually".to_string());
        guard.last_process_probe = None;
        let _ = clear_persisted_agent_pid(app);
        return Ok(());
    }

    if let Some(pid) = guard.tracked_pid.or(read_persisted_agent_pid(app)?) {
        logger.write("INFO", format!("停止已存在的本地 agent 进程，pid={}", pid));
        if is_process_alive(pid)? {
            kill_process(pid)?;
        }
        guard.tracked_pid = None;
        guard.last_exit = Some("agent-run.exe stopped manually".to_string());
        guard.last_process_probe = None;
        let _ = clear_persisted_agent_pid(app);
    }

    Ok(())
}

#[tauri::command]
fn agent_status(
    app: tauri::AppHandle,
    logger: tauri::State<'_, AppLogger>,
    runtime: tauri::State<'_, AgentRuntime>,
) -> Result<AgentStatus, String> {
    let executable_path = local_agent_executable(&app)?;
    let mut guard = runtime
        .0
        .lock()
        .map_err(|_| "无法读取 agent 运行状态。".to_string())?;

    let (running, pid, last_exit) = sync_agent_process_state(&app, &logger, &mut guard)?;

    Ok(AgentStatus {
        running,
        executable_path: executable_path.display().to_string(),
        arguments: vec![],
        pid,
        last_exit,
    })
}

#[tauri::command]
fn start_local_agent(
    app: tauri::AppHandle,
    logger: tauri::State<'_, AppLogger>,
    runtime: tauri::State<'_, AgentRuntime>,
    input: AgentLaunchInput,
) -> Result<AgentStatus, String> {
    let executable_path = resolve_agent_executable(&app, input.executable_path)?;
    let arguments = input.arguments.unwrap_or_default();
    let mut guard = runtime
        .0
        .lock()
        .map_err(|_| "无法锁定 agent 运行状态。".to_string())?;

    let (running, pid, last_exit) = sync_agent_process_state(&app, &logger, &mut guard)?;
    if running {
        if let Some(existing_pid) = pid {
            logger.write("INFO", format!("本地 agent 已在运行，pid={}", existing_pid));
        }
        return Ok(AgentStatus {
            running: true,
            executable_path: executable_path.display().to_string(),
            arguments,
            pid,
            last_exit,
        });
    }
    if last_exit.is_some() {
        logger.write("INFO", "检测到本地 agent 已退出，准备重新拉起");
    }

    if !executable_path.exists() {
        return Err(format!(
            "未找到本地 agent 可执行文件：{}",
            executable_path.display()
        ));
    }

    logger.write(
        "INFO",
        format!(
            "启动本地 agent: {} args={:?}",
            executable_path.display(),
            arguments
        ),
    );
    let mut command = Command::new(&executable_path);
    if !arguments.is_empty() {
        command.args(&arguments);
    }
    configure_background_command(&mut command);
    let child = command.spawn().map_err(|error| error.to_string())?;
    let pid = child.id();
    guard.child = Some(child);
    guard.tracked_pid = Some(pid);
    guard.last_exit = None;
    guard.last_process_probe = None;
    let _ = persist_agent_pid(&app, Some(pid));

    Ok(AgentStatus {
        running: true,
        executable_path: executable_path.display().to_string(),
        arguments,
        pid: Some(pid),
        last_exit: None,
    })
}

#[tauri::command]
fn stop_local_agent(
    app: tauri::AppHandle,
    logger: tauri::State<'_, AppLogger>,
    runtime: tauri::State<'_, AgentRuntime>,
) -> Result<AgentStatus, String> {
    let executable_path = local_agent_executable(&app)?;
    stop_agent_runtime_internal(&app, &logger, &runtime)?;

    let guard = runtime
        .0
        .lock()
        .map_err(|_| "无法锁定 agent 运行状态。".to_string())?;

    Ok(AgentStatus {
        running: false,
        executable_path: executable_path.display().to_string(),
        arguments: vec![],
        pid: None,
        last_exit: guard.last_exit.clone(),
    })
}

#[tauri::command]
fn probe_tcp_endpoint(host: String, port: u16) -> Result<ProbeResult, String> {
    let address = format!("{}:{}", host.trim(), port);
    let target = address
        .to_socket_addrs()
        .map_err(|e| format!("解析地址失败: {}", e))?
        .next()
        .ok_or_else(|| "未解析到可用地址".to_string())?;

    match TcpStream::connect_timeout(&target, Duration::from_secs(3)) {
        Ok(_) => Ok(ProbeResult {
            ok: true,
            address,
            message: "TCP 连接成功".into(),
        }),
        Err(err) => Ok(ProbeResult {
            ok: false,
            address,
            message: format!("TCP 连接失败: {}", err),
        }),
    }
}

#[tauri::command]
fn probe_http_endpoint(url: String) -> Result<ProbeResult, String> {
    let parsed = reqwest::Url::parse(&url).map_err(|e| format!("URL 无效: {}", e))?;
    let host = parsed
        .host_str()
        .ok_or_else(|| "URL 缺少主机名".to_string())?
        .to_string();
    let port = parsed
        .port_or_known_default()
        .ok_or_else(|| "URL 缺少端口信息".to_string())?;
    let address = format!("{}:{}", host, port);
    let target = address
        .to_socket_addrs()
        .map_err(|e| format!("解析地址失败: {}", e))?
        .next()
        .ok_or_else(|| "未解析到可用地址".to_string())?;

    match TcpStream::connect_timeout(&target, Duration::from_secs(3)) {
        Ok(mut stream) => {
            let path = if parsed.path().is_empty() { "/" } else { parsed.path() };
            let request = format!(
                "HEAD {} HTTP/1.1\r\nHost: {}\r\nConnection: close\r\n\r\n",
                path,
                host
            );
            let _ = stream.set_read_timeout(Some(Duration::from_secs(3)));
            let _ = stream.set_write_timeout(Some(Duration::from_secs(3)));
            if let Err(err) = stream.write_all(request.as_bytes()) {
                return Ok(ProbeResult {
                    ok: false,
                    address: url,
                    message: format!("HTTP 写入失败: {}", err),
                });
            }
            let mut buffer = [0u8; 256];
            match stream.read(&mut buffer) {
                Ok(size) if size > 0 => Ok(ProbeResult {
                    ok: true,
                    address: url,
                    message: format!(
                        "HTTP 探测成功: {}",
                        String::from_utf8_lossy(&buffer[..size])
                            .lines()
                            .next()
                            .unwrap_or("响应已返回")
                    ),
                }),
                Ok(_) => Ok(ProbeResult {
                    ok: false,
                    address: url,
                    message: "HTTP 请求未返回数据".into(),
                }),
                Err(err) => Ok(ProbeResult {
                    ok: false,
                    address: url,
                    message: format!("HTTP 读取失败: {}", err),
                }),
            }
        }
        Err(err) => Ok(ProbeResult {
            ok: false,
            address: url,
            message: format!("HTTP 连接失败: {}", err),
        }),
    }
}

#[tauri::command]
fn open_agent_directory(
    app: tauri::AppHandle,
    input: AgentLaunchInput,
) -> Result<(), String> {
    let executable_path = resolve_agent_executable(&app, input.executable_path)?;
    let directory = executable_path
        .parent()
        .ok_or_else(|| "无法定位 agent 所在目录。".to_string())?;
    open_in_file_manager(directory)
}

#[derive(Deserialize)]
struct LogInput {
    level: String,
    message: String,
}

#[tauri::command]
fn log_message(logger: tauri::State<'_, AppLogger>, input: LogInput) -> Result<(), String> {
    logger.write(&input.level, &input.message);
    Ok(())
}

#[tauri::command]
async fn check_for_update(
    app: tauri::AppHandle,
    pending_update: tauri::State<'_, PendingUpdate>,
) -> Result<Option<UpdatePayload>, String> {
    let logger = app.state::<AppLogger>();
    logger.write("INFO", "开始检查更新");

    let pubkey = updater_public_key()
        .ok_or_else(|| "未配置更新公钥，请先在 tauri.conf.json 的 plugins.updater.pubkey 中设置。".to_string())?;
    let endpoints = parsed_updater_endpoints()?;

    if endpoints.is_empty() {
        return Err("未配置更新地址，请先在 tauri.conf.json 的 plugins.updater.endpoints 中设置。".into());
    }

    let update = app
        .updater_builder()
        .pubkey(&pubkey)
        .endpoints(endpoints)
        .map_err(|error| error.to_string())?
        .build()
        .map_err(|error| error.to_string())?
        .check()
        .await
        .map_err(|error| {
            let message = error.to_string();
            logger.write("ERROR", format!("检查更新失败: {message}"));
            message
        })?;

    let payload = update.as_ref().map(|update| UpdatePayload {
        version: update.version.to_string(),
        current_version: update.current_version.to_string(),
        date: update.date.map(|date| date.to_string()),
        body: update.body.clone(),
    });

    let mut pending = pending_update
        .0
        .lock()
        .map_err(|_| "无法锁定待安装更新状态。".to_string())?;
    *pending = update;

    if let Some(update) = pending.as_ref() {
        logger.write(
            "INFO",
            format!(
                "发现新版本: current={}, latest={}",
                update.current_version, update.version
            ),
        );
    } else {
        logger.write("INFO", "未发现可用更新");
    }

    Ok(payload)
}

#[tauri::command]
async fn install_update(
    app: tauri::AppHandle,
    pending_update: tauri::State<'_, PendingUpdate>,
) -> Result<(), String> {
    let logger = app.state::<AppLogger>();
    let update = {
        let mut pending = pending_update
            .0
            .lock()
            .map_err(|_| "无法锁定待安装更新状态。".to_string())?;

        pending
            .take()
            .ok_or_else(|| "当前没有可安装的更新，请先执行一次检查更新。".to_string())?
    };

    logger.write(
        "INFO",
        format!(
            "开始安装更新: current={}, latest={}",
            update.current_version, update.version
        ),
    );

    update
        .download_and_install(|_chunk_length, _content_length| {}, || {})
        .await
        .map_err(|error| {
            let message = error.to_string();
            logger.write("ERROR", format!("安装更新失败: {message}"));
            message
        })?;

    logger.write("INFO", "更新安装完成，准备重启应用");
    app.restart();
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .manage(PendingUpdate::default())
        .manage(AgentRuntime::default())
        .setup(|app| {
            let log_dir = PathBuf::from("D:\\git-projects\\ai-company\\projects\\netunnel\\src\\netunnel-desktop-tauri\\logs");
            let logger = AppLogger::new(log_dir)?;
            logger.write("INFO", "应用启动");
            app.manage(logger);

            #[cfg(desktop)]
            app.handle()
                .plugin(tauri_plugin_updater::Builder::new().build())?;

            let show_item = MenuItemBuilder::with_id("show", "显示主窗口").build(app)?;
            let hide_item = MenuItemBuilder::with_id("hide", "隐藏到托盘").build(app)?;
            let quit_item = MenuItemBuilder::with_id("quit", "退出").build(app)?;
            let menu = MenuBuilder::new(app)
                .item(&show_item)
                .item(&hide_item)
                .separator()
                .item(&quit_item)
                .build()?;

            TrayIconBuilder::new()
                .menu(&menu)
                .show_menu_on_left_click(false)
                .on_menu_event(move |app, event| match event.id.as_ref() {
                    "show" => {
                        let _ = show_main_window(app);
                    }
                    "hide" => {
                        let _ = hide_main_window(app);
                    }
                    "quit" => {
                        let logger = app.state::<AppLogger>();
                        let runtime = app.state::<AgentRuntime>();
                        let _ = stop_agent_runtime_internal(app, &logger, &runtime);
                        app.exit(0);
                    }
                    _ => {}
                })
                .icon(app.default_window_icon().cloned().ok_or("缺少默认图标")?)
                .build(app)?;

            Ok(())
        })
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_prevent_default::init())
        .invoke_handler(tauri::generate_handler![
            logger_status,
            open_logs_directory,
            open_devtools,
            updater_status,
            check_for_update,
            install_update,
            hide_to_tray,
            show_main_window_command,
            agent_status,
            start_local_agent,
            stop_local_agent,
            probe_tcp_endpoint,
            probe_http_endpoint,
            open_agent_directory,
            get_or_create_agent_id,
            reset_agent_id,
            log_message
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
