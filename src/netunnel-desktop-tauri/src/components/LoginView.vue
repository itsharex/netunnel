<script setup lang="ts">
import QRCode from 'qrcode'
import { useWindowControls } from '@/composables/useWindowControls'
import { createWechatLoginSession, pollWechatLoginSessionByUrl } from '@/services/wechat-login'
import type { WechatProfile } from '@/types/auth'
import { onBeforeUnmount, onMounted, ref } from 'vue'

const store = useStore()
const { isWindowMaximized, minimizeWindow, toggleMaximizeWindow, closeWindow } = useWindowControls()

const APP_NAME = '网跃通'
const POLL_INTERVAL_MS = 2500

const bizId = ref('')
const pollUrl = ref('')
const qrCodeDataUrl = ref('')
const qrCodeUrl = ref('')
const scanStatus = ref<'idle' | 'pending' | 'success' | 'expired' | 'error'>('idle')
const scanMessage = ref('正在准备微信登录二维码...')
const expiresAt = ref('')
let pollTimer: ReturnType<typeof setInterval> | null = null

function generateBizId() {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return `netunnel_${crypto.randomUUID()}`
  }
  return `netunnel_${Date.now()}_${Math.random().toString(36).slice(2, 10)}`
}

function stopPolling() {
  if (!pollTimer) {
    return
  }
  clearInterval(pollTimer)
  pollTimer = null
}

async function renderQRCode(url: string) {
  qrCodeDataUrl.value = await QRCode.toDataURL(url, {
    width: 220,
    margin: 1,
  })
}

async function handleWechatSuccess(profile: WechatProfile) {
  stopPolling()
  scanStatus.value = 'success'
  scanMessage.value = `微信已授权，正在登录 ${APP_NAME}...`
  await store.loginWithWechatProfile(profile)
}

async function pollLoginStatus() {
  if (!bizId.value || !pollUrl.value || scanStatus.value === 'success') {
    return
  }

  try {
    const session = await pollWechatLoginSessionByUrl(pollUrl.value)
    expiresAt.value = session.expiresAt
    scanStatus.value = session.status

    if (session.status === 'pending') {
      scanMessage.value = '请使用微信公众号扫码并在手机端确认授权。'
      return
    }

    if (session.status === 'expired') {
      stopPolling()
      scanMessage.value = '二维码已过期，请重新生成。'
      return
    }

    if (session.status === 'success' && session.profile) {
      await handleWechatSuccess(session.profile)
    }
  } catch (error) {
    stopPolling()
    scanStatus.value = 'error'
    scanMessage.value = String(error)
  }
}

async function startWechatLogin() {
  stopPolling()
  store.loginError = ''
  qrCodeDataUrl.value = ''
  qrCodeUrl.value = ''
  pollUrl.value = ''
  scanStatus.value = 'idle'
  scanMessage.value = '正在生成微信登录二维码...'

  try {
    const nextBizId = generateBizId()
    bizId.value = nextBizId
    const session = await createWechatLoginSession(nextBizId)
    qrCodeUrl.value = session.qrCodeUrl
    pollUrl.value = session.pollUrl
    expiresAt.value = session.expiresAt
    scanStatus.value = session.status
    await renderQRCode(session.qrCodeUrl)
    scanMessage.value = '请使用微信公众号扫码并在手机端确认授权。'

    pollTimer = setInterval(() => {
      void pollLoginStatus()
    }, POLL_INTERVAL_MS)
  } catch (error) {
    scanStatus.value = 'error'
    scanMessage.value = String(error)
  }
}

onMounted(() => {
  void startWechatLogin()
})

onBeforeUnmount(() => {
  stopPolling()
})
</script>

<template>
  <main class="window-drag-region flex min-h-screen justify-center overflow-hidden bg-[linear-gradient(135deg,#eef1ff_0%,#f8eef6_32%,#dff3ff_100%)] px-4 pt-3 pb-4" data-tauri-drag-region>
    <div class="flex w-full max-w-[320px] flex-col items-center">
      <div class="mb-4 flex h-7 w-full items-center justify-end" data-tauri-drag-region>
        <div class="window-controls" style="-webkit-app-region: no-drag">
        <button class="window-control-button login-window-button" type="button" @click="minimizeWindow">
          <span class="i-mdi-window-minimize"></span>
        </button>
        <button class="window-control-button login-window-button" type="button" @click="toggleMaximizeWindow">
          <span :class="isWindowMaximized ? 'i-mdi-window-restore' : 'i-mdi-checkbox-blank-outline'"></span>
        </button>
        <button class="window-control-button login-window-button window-control-button--close" type="button" @click="closeWindow">
          <span class="i-mdi-close"></span>
        </button>
        </div>
      </div>

      <div class="flex w-full flex-col items-center text-center">
        <div class="mb-7 flex items-center justify-center gap-1 text-[34px] font-semibold leading-none tracking-[0.08em]">
          <span class="bg-[linear-gradient(180deg,#67d5ff,#4d9eff)] bg-clip-text text-transparent">恒</span>
          <span class="bg-[linear-gradient(180deg,#96a5ff,#d77cff)] bg-clip-text text-transparent">易兴</span>
        </div>

        <p class="mb-7 text-[16px] font-medium text-[#111111]">手机微信扫码登录</p>

        <button
          class="mb-6 flex h-[198px] w-[198px] items-center justify-center rounded-[10px] bg-white p-[10px] shadow-[0_10px_30px_rgba(118,143,177,0.18)]"
          type="button"
          :disabled="store.isLoginSubmitting"
          @click="startWechatLogin"
        >
          <img v-if="qrCodeDataUrl" :src="qrCodeDataUrl" alt="微信登录二维码" class="h-full w-full object-contain" />
          <div v-else class="text-sm text-[var(--text-soft)]">二维码生成中...</div>
        </button>

        <p v-if="store.loginError" class="max-w-[240px] text-center text-xs text-rose-600">
          {{ store.loginError }}
        </p>
      </div>
    </div>
  </main>
</template>
