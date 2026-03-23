<script setup lang="ts">
import { computed, onMounted } from "vue";
import { RouterLink, RouterView, useRoute } from "vue-router";
import { useWorkspaceState } from "./state";

const state = useWorkspaceState();
const route = useRoute();

const navItems = [
  { to: "/session", label: "Session" },
  { to: "/dashboard", label: "Dashboard" },
  { to: "/tunnels", label: "Tunnels" },
  { to: "/usage", label: "Usage" },
  { to: "/billing", label: "Billing" },
];

const routeTitle = computed(() => {
  if (route.path.startsWith("/session")) {
    return "会话面板";
  }
  if (route.path.startsWith("/tunnels")) {
    return "Tunnel 面板";
  }
  if (route.path.startsWith("/billing")) {
    return "账务面板";
  }
  if (route.path.startsWith("/usage")) {
    return "流量面板";
  }
  return "运行概览";
});

onMounted(() => {
  void state.loadAll();
});
</script>

<template>
  <div class="shell">
    <aside class="hero">
      <p class="eyebrow">Netunnel Desktop</p>
      <h1>内网穿透控制台</h1>
      <p class="hero-copy">
        这是一套先对接后端、后续再平移进 Tauri 模板的桌面端原型。当前结构已经按正式桌面应用拆成概览、隧道和账务三个区域。
      </p>

      <nav class="nav-stack">
        <RouterLink
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          class="nav-link"
          active-class="nav-link-active"
        >
          {{ item.label }}
        </RouterLink>
      </nav>

      <div class="connection-panel">
        <div class="session-chip-grid">
          <div class="session-chip">
            <span>Session</span>
            <strong>{{ state.sessionSummary.value.mode }}</strong>
          </div>
          <div class="session-chip">
            <span>User</span>
            <strong>{{ state.sessionSummary.value.nickname }}</strong>
          </div>
        </div>
        <label>
          <span>API Base URL</span>
          <input v-model="state.baseUrl.value" />
        </label>
        <label>
          <span>User ID</span>
          <input v-model="state.userId.value" />
        </label>
        <label>
          <span>Access Token</span>
          <input v-model="state.accessToken.value" placeholder="future bearer token" />
        </label>
        <button class="primary" :disabled="state.loading.value" @click="state.loadAll">
          {{ state.loading.value ? "同步中..." : "刷新数据" }}
        </button>
      </div>

      <div class="status-box" v-if="state.actionMessage.value || state.actionError.value">
        <p v-if="state.actionMessage.value" class="status-ok">{{ state.actionMessage.value }}</p>
        <p v-if="state.actionError.value" class="status-error">{{ state.actionError.value }}</p>
      </div>
    </aside>

    <main class="board">
      <section class="page-head">
        <div>
          <p class="page-kicker">Workspace</p>
          <h2>{{ routeTitle }}</h2>
        </div>
        <button class="ghost" :disabled="state.loading.value" @click="state.loadAll">重新同步</button>
      </section>

      <RouterView />
    </main>
  </div>
</template>
