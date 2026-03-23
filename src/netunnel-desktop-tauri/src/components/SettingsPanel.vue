<script setup lang="ts">
const props = defineProps<{
  mode: 'modal'
}>()

const emit = defineEmits<{
  close: []
}>()

const store = useStore()

const closePanel = () => emit('close')

const navSections = [
  { id: 'general', label: '通用', icon: 'cog-outline' },
  { id: 'appearance', label: '外观', icon: 'palette-outline' },
  { id: 'network', label: '服务连接', icon: 'web' },
  { id: 'privacy', label: '安全与更新', icon: 'shield-outline' },
  { id: 'downloads', label: '日志与调试', icon: 'download' },
] as const

type SectionId = (typeof navSections)[number]['id']

const themeCards = [
  { value: 'light', label: '浅色', preview: 'light' },
  { value: 'dark', label: '深色', preview: 'dark' },
  { value: 'system', label: '跟随系统', preview: 'system' },
] as const

const activeSection = ref<SectionId>('appearance')
const contentBodyRef = ref<HTMLElement | null>(null)
const sectionRefs = ref<Record<SectionId, HTMLElement | null>>({
  general: null,
  appearance: null,
  network: null,
  privacy: null,
  downloads: null,
})

const assignSectionRef = (id: SectionId) => (element: Element | ComponentPublicInstance | null) => {
  sectionRefs.value[id] = element as HTMLElement | null
}

const updaterDescription = computed(() => {
  if (store.updater.installing) {
    return '正在下载并安装更新，请稍候。安装完成后应用会自动重启。'
  }

  if (store.updater.checking) {
    return '正在检查 GitHub Release 中的新版本，请稍候。'
  }

  if (store.updater.available) {
    return `发现新版本 v${store.updater.available.version}，下载安装后应用会自动重启。`
  }

  if (store.updater.lastError) {
    return `更新检查失败：${store.updater.lastError}`
  }

  if (store.updater.reason) {
    return store.updater.reason
  }

  if (store.updater.lastCheckedAt) {
    return `最近一次检查：${new Date(store.updater.lastCheckedAt).toLocaleString()}`
  }

  return '检查 GitHub Release 中的新版本并安装。'
})

const updaterButtonLabel = computed(() => {
  if (store.updater.installing) {
    return '正在安装...'
  }

  if (store.updater.checking) {
    return '检查中...'
  }

  if (store.updater.available) {
    return `安装 v${store.updater.available.version}`
  }

  return '检查更新'
})

const handleUpdaterAction = async () => {
  if (store.updater.available) {
    await store.installAvailableUpdate()
    return
  }

  await store.checkForUpdates()
}

const syncActiveSection = () => {
  const container = contentBodyRef.value
  if (!container) {
    return
  }

  const containerTop = container.getBoundingClientRect().top
  let current: SectionId = navSections[0].id
  let smallestOffset = Number.POSITIVE_INFINITY

  for (const section of navSections) {
    const element = sectionRefs.value[section.id]
    if (!element) {
      continue
    }

    const offset = Math.abs(element.getBoundingClientRect().top - containerTop - 24)
    if (offset < smallestOffset) {
      smallestOffset = offset
      current = section.id
    }
  }

  activeSection.value = current
}

const scrollToSection = (id: SectionId) => {
  const container = contentBodyRef.value
  const element = sectionRefs.value[id]
  if (!container || !element) {
    return
  }

  activeSection.value = id
  container.scrollTo({
    top: element.offsetTop - 20,
    behavior: 'smooth',
  })
}

onMounted(() => {
  nextTick(() => {
    syncActiveSection()
  })
})
</script>

<template>
  <section :class="props.mode === 'modal' ? 'settings-modal-shell settings-modal-shell--design' : 'settings-modal-shell'">
    <aside class="settings-sidebar settings-sidebar--modal">
      <div class="flex items-center gap-3 mb-2">
        <div class="size-8 rounded-lg bg-[var(--brand)] flex items-center justify-center text-white">
          <span class="i-mdi-cog-outline text-xl"></span>
        </div>
        <h1 class="text-lg font-bold text-[var(--text-strong)]">设置</h1>
      </div>

      <nav class="flex flex-col gap-1 flex-1">
        <button
          v-for="section in navSections"
          :key="section.id"
          class="settings-nav-item settings-nav-item--design"
          :class="{ 'is-active': activeSection === section.id }"
          type="button"
          @click="scrollToSection(section.id)"
        >
          <span :class="`i-mdi-${section.icon}`" class="text-[18px]"></span>
          <span class="text-sm font-medium">{{ section.label }}</span>
        </button>
      </nav>

      <div class="mt-auto pt-6 border-t border-[var(--line)]">
        <button class="settings-nav-item settings-nav-item--design text-[var(--text-muted)]" type="button">
          <span class="i-mdi-information-outline text-[18px]"></span>
          <span class="text-xs font-bold uppercase tracking-wider">关于 netunnel</span>
        </button>
      </div>
    </aside>

    <div class="settings-content-shell">
      <header class="settings-content-header">
        <button class="settings-close-button" type="button" @click="closePanel">
          <span class="i-mdi-close text-[var(--text-muted)]"></span>
        </button>
      </header>

      <div ref="contentBodyRef" class="settings-content-body" @scroll="syncActiveSection">
        <section :ref="assignSectionRef('general')" class="settings-block">
          <div class="settings-block__header">
            <div>
              <h2 class="settings-block__title">通用</h2>
              <p class="settings-block__desc">应用启动、桌面行为和基础运行设置。</p>
            </div>
          </div>
          <div class="settings-stack">
            <div class="settings-card settings-card--form">
              <div class="flex items-start justify-between gap-4">
                <div class="space-y-2">
                  <div class="flex items-center gap-2">
                    <h3 class="settings-subhead">应用更新</h3>
                    <span
                      class="rounded-full px-2 py-1 text-[10px] font-semibold"
                      :class="
                        store.updater.available
                          ? 'bg-emerald-500/15 text-emerald-600'
                          : store.updater.enabled
                            ? 'bg-sky-500/15 text-sky-600'
                            : 'bg-slate-500/15 text-slate-500'
                      "
                    >
                      {{ store.updater.available ? '有新版本' : store.updater.enabled ? '已启用' : '未启用' }}
                    </span>
                  </div>
                  <p class="settings-inline-desc">当前版本 {{ store.version }}</p>
                  <p class="text-sm leading-6 text-[var(--text-soft)]">
                    {{ updaterDescription }}
                  </p>
                  <p v-if="store.updater.available?.body" class="rounded-2xl bg-[var(--surface-secondary)] px-4 py-3 text-sm leading-6 text-[var(--text-soft)]">
                    {{ store.updater.available.body }}
                  </p>
                </div>

                <button
                  class="settings-save-button shrink-0"
                  :disabled="(!store.updater.enabled && !store.updater.available) || store.updater.checking || store.updater.installing"
                  type="button"
                  @click="handleUpdaterAction"
                >
                  {{ updaterButtonLabel }}
                </button>
              </div>
            </div>
            <div class="settings-card-row">
              <div>
                <h3 class="settings-subhead">启动行为</h3>
                <p class="settings-inline-desc">启动应用时自动恢复上次的 netunnel 工作区状态。</p>
              </div>
              <label class="switch">
                <input :checked="store.settings.launchAtStartup" type="checkbox" @change="store.updateSetting('launchAtStartup', ($event.target as HTMLInputElement).checked)" />
                <span class="switch-ui"></span>
              </label>
            </div>
            <div class="settings-card-row">
              <div>
                <h3 class="settings-subhead">关闭到托盘</h3>
                <p class="settings-inline-desc">点击右上角关闭按钮时不退出应用，而是隐藏到系统托盘。</p>
              </div>
              <label class="switch">
                <input :checked="store.settings.closeToTray" type="checkbox" @change="store.updateSetting('closeToTray', ($event.target as HTMLInputElement).checked)" />
                <span class="switch-ui"></span>
              </label>
            </div>
            <div class="settings-card settings-card--form">
              <div>
                <h3 class="settings-subhead">默认 API 地址</h3>
                <p class="settings-inline-desc">用于新工作区初始化时的服务端管理 API 地址。</p>
              </div>
              <input :value="store.settings.homeUrl" class="text-input settings-text-input" type="text" @input="store.updateSetting('homeUrl', ($event.target as HTMLInputElement).value)" />
            </div>
            <div class="settings-card settings-card--form">
              <div>
                <h3 class="settings-subhead">默认 Bridge 地址</h3>
                <p class="settings-inline-desc">用于会话页生成本地 agent 启动参数的默认桥接地址。</p>
              </div>
              <input :value="store.settings.bridgeAddr" class="text-input settings-text-input" type="text" @input="store.updateSetting('bridgeAddr', ($event.target as HTMLInputElement).value)" />
            </div>
            <div class="settings-card settings-card--form">
              <div>
                <h3 class="settings-subhead">默认 Agent 路径</h3>
                <p class="settings-inline-desc">预填本地 agent 可执行路径，方便桌面端直接启停。</p>
              </div>
              <input :value="store.settings.agentExecutablePath" class="text-input settings-text-input" type="text" @input="store.updateSetting('agentExecutablePath', ($event.target as HTMLInputElement).value)" />
            </div>
            <div class="settings-card settings-card--form">
              <div>
                <h3 class="settings-subhead">默认同步间隔</h3>
                <p class="settings-inline-desc">用于生成 agent 启动参数时的默认同步周期，单位秒。</p>
              </div>
              <input :value="store.settings.defaultSyncInterval" class="text-input settings-text-input" type="text" @input="store.updateSetting('defaultSyncInterval', ($event.target as HTMLInputElement).value)" />
            </div>
            <div class="settings-card settings-card--form">
              <div class="flex items-start justify-between gap-4">
                <div class="space-y-2">
                  <h3 class="settings-subhead">日志文件</h3>
                  <p class="settings-inline-desc">应用会将启动、检查更新和安装更新等信息写入本地日志，便于排查问题。</p>
                  <p class="text-sm leading-6 text-[var(--text-soft)]">
                    {{ store.logs.filePath ?? store.logs.lastError ?? '正在读取日志目录...' }}
                  </p>
                </div>

                <button class="settings-save-button shrink-0" type="button" @click="store.openLogsDirectory()">
                  打开日志目录
                </button>
              </div>
            </div>
          </div>
        </section>

        <section :ref="assignSectionRef('appearance')" class="settings-block">
          <div class="settings-block__header">
            <div>
              <h2 class="settings-block__title">外观</h2>
              <p class="settings-block__desc">个性化您的桌面控制台界面，调整颜色、圆角和透明度。</p>
            </div>
          </div>
          <div class="settings-stack">
            <section class="space-y-4">
              <h3 class="settings-subhead">主题模式</h3>
              <div class="grid grid-cols-3 gap-3">
                <button
                  v-for="card in themeCards"
                  :key="card.value"
                  class="theme-card theme-card--design"
                  :class="{ 'is-selected': store.settings.theme === card.value }"
                  type="button"
                  @click="store.updateTheme(card.value)"
                >
                  <div class="theme-preview" :class="`theme-preview--${card.preview}`">
                    <template v-if="card.preview !== 'system'">
                      <div class="theme-preview-line"></div>
                      <div class="theme-preview-box"></div>
                    </template>
                    <template v-else>
                      <div class="theme-preview-half theme-preview-half--light">
                        <div class="theme-preview-line"></div>
                        <div class="theme-preview-box"></div>
                      </div>
                      <div class="theme-preview-half theme-preview-half--dark">
                        <div class="theme-preview-line"></div>
                        <div class="theme-preview-box"></div>
                      </div>
                    </template>
                  </div>
                  <div class="flex items-center justify-between">
                    <span class="text-sm font-medium text-[var(--text-strong)]">{{ card.label }}</span>
                    <div class="theme-radio" :class="{ 'is-selected': store.settings.theme === card.value }">
                      <div></div>
                    </div>
                  </div>
                </button>
              </div>
            </section>

            <section class="space-y-4">
              <div class="flex items-center justify-between">
                <div>
                  <h3 class="settings-subhead">Acrylic 亚克力效果</h3>
                  <p class="settings-inline-desc">开启背景半透明磨砂质感 (需系统支持)</p>
                </div>
                <label class="switch">
                  <input v-model="store.settings.acrylicEnabled" type="checkbox" />
                  <span class="switch-ui"></span>
                </label>
              </div>
              <div class="space-y-2">
                <div class="flex justify-between text-xs mb-1">
                  <span class="text-[var(--text-soft)]">透明度</span>
                  <span class="font-medium text-[var(--text-strong)]">{{ store.settings.transparency }}%</span>
                </div>
                <input
                  class="slider"
                  :value="store.settings.transparency"
                  max="100"
                  min="50"
                  type="range"
                  @input="store.setTransparency(Number(($event.target as HTMLInputElement).value))"
                />
              </div>
            </section>
          </div>
        </section>

        <section :ref="assignSectionRef('network')" class="settings-block">
          <div class="settings-block__header">
            <div>
              <h2 class="settings-block__title">服务连接</h2>
              <p class="settings-block__desc">配置服务端连接、接口地址和未来的代理策略预留。</p>
            </div>
          </div>
          <div class="settings-stack">
            <div class="settings-card-row">
              <div>
                <h3 class="settings-subhead">自动刷新状态</h3>
                <p class="settings-inline-desc">预留为自动刷新隧道、连接与计费状态的开关。</p>
              </div>
              <label class="switch">
                <input checked type="checkbox" />
                <span class="switch-ui"></span>
              </label>
            </div>
            <div class="settings-card settings-card--form">
              <div>
                <h3 class="settings-subhead">服务端入口</h3>
                <p class="settings-inline-desc">这里直接复用桌面端的默认 API 地址，会同步影响会话页初始化。</p>
              </div>
              <input :value="store.settings.homeUrl" class="text-input settings-text-input" type="text" @input="store.updateSetting('homeUrl', ($event.target as HTMLInputElement).value)" />
            </div>
            <div class="settings-card settings-card--form">
              <div>
                <h3 class="settings-subhead">Bridge 地址</h3>
                <p class="settings-inline-desc">为本地 agent 启动参数提供统一默认值，减少重复录入。</p>
              </div>
              <input :value="store.settings.bridgeAddr" class="text-input settings-text-input" type="text" @input="store.updateSetting('bridgeAddr', ($event.target as HTMLInputElement).value)" />
            </div>
          </div>
        </section>

        <section :ref="assignSectionRef('privacy')" class="settings-block">
          <div class="settings-block__header">
            <div>
              <h2 class="settings-block__title">安全与更新</h2>
              <p class="settings-block__desc">围绕桌面端安全、更新策略和本地运行保护的配置预留。</p>
            </div>
          </div>
          <div class="settings-stack">
            <div class="settings-card-row">
              <div>
                <h3 class="settings-subhead">更新提醒</h3>
                <p class="settings-inline-desc">有新版本时显示桌面端更新提醒，并允许手动安装。</p>
              </div>
              <label class="switch">
                <input checked type="checkbox" />
                <span class="switch-ui"></span>
              </label>
            </div>
          </div>
        </section>

        <section :ref="assignSectionRef('downloads')" class="settings-block">
          <div class="settings-block__header">
            <div>
              <h2 class="settings-block__title">日志与调试</h2>
              <p class="settings-block__desc">日志目录、调试信息和桌面端排障相关配置。</p>
            </div>
          </div>
          <div class="settings-stack">
            <div class="settings-card settings-card--form">
              <div>
                <h3 class="settings-subhead">默认日志位置</h3>
                <p class="settings-inline-desc">桌面端当前使用 Tauri 本地日志目录记录启动、更新和运行信息。</p>
              </div>
              <input class="text-input settings-text-input" :value="store.logs.filePath ?? '正在读取日志路径...'" type="text" />
            </div>
            <div class="settings-card-row">
              <div>
                <h3 class="settings-subhead">调试模式提示</h3>
                <p class="settings-inline-desc">开发环境下允许继续使用 `F12` 打开 DevTools 进行联调。</p>
              </div>
              <label class="switch">
                <input checked type="checkbox" />
                <span class="switch-ui"></span>
              </label>
            </div>
          </div>
        </section>
      </div>

      <footer class="settings-footer">
        <button class="settings-cancel-button" type="button" @click="closePanel">取消</button>
        <button class="settings-save-button" type="button" @click="closePanel">保存更改</button>
      </footer>
    </div>
  </section>
</template>
