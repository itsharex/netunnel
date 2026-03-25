<script setup lang="ts">
import DashboardView from '@/components/DashboardView.vue'
import LoginView from '@/components/LoginView.vue'
import { resizeMainWindow, showMainWindow } from '@/services/window'
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const store = useStore()
store.initApp()

const LOGIN_WINDOW_WIDTH = 360
const LOGIN_WINDOW_HEIGHT = 520
const DASHBOARD_WINDOW_WIDTH = 993
const DASHBOARD_WINDOW_HEIGHT = 728
const activeView = ref<'login' | 'dashboard'>(store.isAuthenticated ? 'dashboard' : 'login')
const isSwitchingView = ref(false)
let windowStateRequestId = 0

const handleKeydown = (event: KeyboardEvent) => {
  if (event.key !== 'F12') {
    return
  }

  event.preventDefault()
  void store.openDevtools()
}

const resizeWindowForAuthenticatedView = async () => {
  await resizeMainWindow(DASHBOARD_WINDOW_WIDTH, DASHBOARD_WINDOW_HEIGHT)
}

const resizeWindowForLoginView = async () => {
  await nextTick()
  await resizeMainWindow(LOGIN_WINDOW_WIDTH, LOGIN_WINDOW_HEIGHT)
}

const applyWindowState = async (isAuthenticated: boolean) => {
  const requestId = ++windowStateRequestId
  isSwitchingView.value = true

  if (isAuthenticated) {
    await resizeWindowForAuthenticatedView()
    if (requestId !== windowStateRequestId) {
      return
    }
    activeView.value = 'dashboard'
    await nextTick()
  } else {
    activeView.value = 'login'
    await nextTick()
    await resizeWindowForLoginView()
    if (requestId !== windowStateRequestId) {
      return
    }
  }

  await showMainWindow()
  if (requestId === windowStateRequestId) {
    isSwitchingView.value = false
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
  store.stopAutoUpdateChecks()
})

watch(
  () => store.isAuthenticated,
  (isAuthenticated) => {
    if (isAuthenticated) {
      store.startAutoUpdateChecks()
      void applyWindowState(true)
      return
    }

    store.stopAutoUpdateChecks()
    void applyWindowState(false)
  },
  { immediate: true },
)
</script>

<template>
  <div
    class="app-shell min-h-screen relative overflow-hidden"
    :class="[store.themeClass, { 'has-acrylic': store.settings.acrylicEnabled }]"
    :style="{
      '--ui-radius': `${store.currentRadius}px`,
      '--ui-transparency': `${store.settings.transparency / 100}`,
    }"
  >
    <transition name="app-view" mode="out-in">
      <LoginView v-if="activeView === 'login'" key="login" />
      <DashboardView v-else key="dashboard" />
    </transition>
    <div v-if="isSwitchingView" class="app-transition-mask" />
  </div>
</template>

<style scoped>
.app-view-enter-active,
.app-view-leave-active {
  transition:
    opacity 0.18s ease,
    transform 0.18s ease;
}

.app-view-enter-from,
.app-view-leave-to {
  opacity: 0;
  transform: scale(0.985);
}

.app-transition-mask {
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.08), rgba(255, 255, 255, 0));
  opacity: 0.55;
}

.theme-dark .app-transition-mask {
  background: linear-gradient(180deg, rgba(15, 23, 42, 0.3), rgba(15, 23, 42, 0.06));
}
</style>
