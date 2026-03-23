<script setup lang="ts">
import DashboardView from '@/components/DashboardView.vue'
import LoginView from '@/components/LoginView.vue'
import { resizeMainWindow, showMainWindow } from '@/services/window'
import { nextTick, onBeforeUnmount, onMounted, watch } from 'vue'

const store = useStore()
store.initApp()

const LOGIN_WINDOW_WIDTH = 360
const LOGIN_WINDOW_HEIGHT = 520
const DASHBOARD_WINDOW_WIDTH = 993
const DASHBOARD_WINDOW_HEIGHT = 728

const handleKeydown = (event: KeyboardEvent) => {
  if (event.key !== 'F12') {
    return
  }

  event.preventDefault()
  void store.openDevtools()
}

const resizeWindowForAuthenticatedView = async () => {
  await nextTick()
  await new Promise((resolve) => window.setTimeout(resolve, 80))
  await resizeMainWindow(DASHBOARD_WINDOW_WIDTH, DASHBOARD_WINDOW_HEIGHT)
}

const resizeWindowForLoginView = async () => {
  await nextTick()
  await resizeMainWindow(LOGIN_WINDOW_WIDTH, LOGIN_WINDOW_HEIGHT)
}

const applyWindowState = async (isAuthenticated: boolean) => {
  if (isAuthenticated) {
    await resizeWindowForAuthenticatedView()
  } else {
    await resizeWindowForLoginView()
  }
  await showMainWindow()
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
    class="app-shell min-h-screen"
    :class="[store.themeClass, { 'has-acrylic': store.settings.acrylicEnabled }]"
    :style="{
      '--ui-radius': `${store.currentRadius}px`,
      '--ui-transparency': `${store.settings.transparency / 100}`,
    }"
  >
    <LoginView v-if="!store.isAuthenticated" />
    <DashboardView v-else />
  </div>
</template>
