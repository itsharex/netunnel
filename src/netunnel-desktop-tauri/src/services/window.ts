import { invoke, isTauri } from '@tauri-apps/api/core'
import { LogicalSize } from '@tauri-apps/api/dpi'
import { getCurrentWindow } from '@tauri-apps/api/window'

function getSafeCurrentWindow() {
  if (!isTauri()) {
    return null
  }

  try {
    return getCurrentWindow()
  } catch (error) {
    console.error('[window] failed to get current window', error)
    return null
  }
}

export async function resizeMainWindow(width: number, height: number) {
  const currentWindow = getSafeCurrentWindow()
  if (!currentWindow) {
    console.warn('[window] resize skipped: current window unavailable')
    return false
  }

  try {
    const nextSize = new LogicalSize(width, height)
    console.info('[window] resizing main window', { width, height })
    await currentWindow.setMinSize(nextSize)
    await currentWindow.setSize(nextSize)
    await currentWindow.center()
    return true
  } catch (error) {
    console.error('[window] resize failed', error)
    return false
  }
}

export async function showMainWindow() {
  if (!isTauri()) {
    return false
  }

  try {
    await invoke('show_main_window_command')
    return true
  } catch (error) {
    console.error('[window] show failed', error)
    return false
  }
}
