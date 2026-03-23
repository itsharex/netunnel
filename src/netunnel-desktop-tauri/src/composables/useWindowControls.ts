import { getCurrentWindow } from '@tauri-apps/api/window'
import { invoke, isTauri } from '@tauri-apps/api/core'
import { useStore } from '@/store'

export function useWindowControls() {
  const store = useStore()
  const isWindowMaximized = ref(false)
  let unlistenResize: null | (() => void) = null
  const getSafeCurrentWindow = () => {
    if (!isTauri()) {
      return null
    }

    try {
      return getCurrentWindow()
    } catch {
      return null
    }
  }

  const syncWindowState = async () => {
    const currentWindow = getSafeCurrentWindow()
    if (!currentWindow) {
      isWindowMaximized.value = false
      return
    }

    try {
      isWindowMaximized.value = await currentWindow.isMaximized()
    } catch {
      isWindowMaximized.value = false
    }
  }

  const minimizeWindow = async () => {
    const currentWindow = getSafeCurrentWindow()
    if (!currentWindow) {
      return
    }

    await currentWindow.minimize()
  }

  const toggleMaximizeWindow = async () => {
    const currentWindow = getSafeCurrentWindow()
    if (!currentWindow) {
      return
    }

    const maximized = await currentWindow.isMaximized()

    if (maximized) {
      await currentWindow.unmaximize()
    } else {
      await currentWindow.maximize()
    }

    await syncWindowState()
  }

  const closeWindow = async () => {
    if (isTauri() && store.settings.closeToTray) {
      await invoke('hide_to_tray')
      return
    }

    const currentWindow = getSafeCurrentWindow()
    if (!currentWindow) {
      return
    }

    await currentWindow.close()
  }

  const startDraggingWindow = async () => {
    const currentWindow = getSafeCurrentWindow()
    if (!currentWindow) {
      return
    }

    await currentWindow.startDragging()
  }

  onMounted(async () => {
    await syncWindowState()
    const currentWindow = getSafeCurrentWindow()
    if (!currentWindow) {
      return
    }

    unlistenResize = await currentWindow.onResized(async () => {
      await syncWindowState()
    })
  })

  onBeforeUnmount(() => {
    unlistenResize?.()
  })

  return {
    isWindowMaximized,
    minimizeWindow,
    toggleMaximizeWindow,
    closeWindow,
    startDraggingWindow,
  }
}
