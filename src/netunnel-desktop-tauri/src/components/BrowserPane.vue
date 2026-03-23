<script setup lang="ts">
defineProps<{
  pane: {
    id: number
    title: string
    url: string
    summary: string
    tag: string
    accent: string
    status: 'online' | 'syncing'
  }
}>()
</script>

<template>
  <article class="pane-card pane-card--flat">
    <header class="pane-toolbar pane-toolbar--flat">
      <div class="flex gap-1">
        <button class="pane-tool-button pane-tool-button--flat"><span class="i-mdi-arrow-left text-sm"></span></button>
        <button class="pane-tool-button pane-tool-button--flat"><span class="i-mdi-arrow-right text-sm"></span></button>
        <button class="pane-tool-button pane-tool-button--flat"><span class="i-mdi-refresh text-sm"></span></button>
      </div>

      <span class="text-xs font-semibold text-[var(--text-strong)] flex-shrink-0 ml-1">{{ pane.title }}</span>

      <div class="pane-address-bar">
        <span class="i-mdi-lock-outline text-xs text-green-500"></span>
        <span class="text-xs truncate text-[var(--text-soft)]">{{ pane.url }}</span>
      </div>

      <button class="pane-tool-button pane-tool-button--flat"><span class="i-mdi-dots-vertical text-sm"></span></button>
    </header>

    <div class="flex-1 bg-slate-100 dark:bg-slate-800 overflow-hidden">
      <div class="pane-preview pane-preview--flat" :class="pane.accent">
        <div class="pane-preview-grid"></div>
        <div class="relative z-10 h-full w-full flex flex-col justify-end p-6">
          <div class="max-w-sm rounded-2xl bg-white/70 p-4 backdrop-blur">
            <div class="text-[10px] font-semibold uppercase tracking-wider text-slate-500">{{ pane.tag }}</div>
            <div class="mt-2 text-base font-semibold text-slate-900">{{ pane.title }}</div>
            <div class="mt-1 text-xs leading-5 text-slate-600">{{ pane.summary }}</div>
          </div>
        </div>
      </div>
    </div>
  </article>
</template>
