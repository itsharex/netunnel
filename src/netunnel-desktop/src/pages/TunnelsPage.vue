<script setup lang="ts">
import { computed, onMounted } from "vue";
import { useWorkspaceState } from "../state";

const state = useWorkspaceState();
const tunnels = computed(() => state.tunnels.value);
const routes = computed(() => state.domainRoutes.value);

onMounted(async () => {
  if (tunnels.value.length === 0) {
    await state.loadAll();
  } else {
    await state.reloadDomainRoutes();
  }
  state.syncDefaultAgentId();
});
</script>

<template>
  <div class="page-stack">
    <section class="panel">
      <div class="panel-head">
        <h2>创建 TCP Tunnel</h2>
        <p>适合数据库、SSH 和其他纯 TCP 服务暴露。</p>
      </div>

      <form class="inline-form two-columns" @submit.prevent="state.createTcp">
        <label>
          <span>Agent ID</span>
          <input v-model="state.tcpForm.value.agentId" />
        </label>
        <label>
          <span>名称</span>
          <input v-model="state.tcpForm.value.name" />
        </label>
        <label>
          <span>本地 Host</span>
          <input v-model="state.tcpForm.value.localHost" />
        </label>
        <label>
          <span>本地 Port</span>
          <input v-model="state.tcpForm.value.localPort" />
        </label>
        <label>
          <span>远端 Port</span>
          <input v-model="state.tcpForm.value.remotePort" />
        </label>
        <div class="form-action-cell">
          <button class="accent" :disabled="state.loading.value">创建 TCP Tunnel</button>
        </div>
      </form>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>创建 HTTP / HTTPS Host Tunnel</h2>
        <p>域名后缀和 HTTPS 证书由服务端统一托管。</p>
      </div>

      <form class="inline-form two-columns" @submit.prevent="state.createHttpHost">
        <label>
          <span>Agent ID</span>
          <input v-model="state.hostForm.value.agentId" />
        </label>
        <label>
          <span>名称</span>
          <input v-model="state.hostForm.value.name" />
        </label>
        <label>
          <span>本地 Host</span>
          <input v-model="state.hostForm.value.localHost" />
        </label>
        <label>
          <span>本地 Port</span>
          <input v-model="state.hostForm.value.localPort" />
        </label>
        <label>
          <span>域名前缀</span>
          <input v-model="state.hostForm.value.domain" />
        </label>
        <div class="form-action-cell">
          <button class="accent" :disabled="state.loading.value">创建 Host Tunnel</button>
        </div>
      </form>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>Tunnel 管理</h2>
        <p>当前支持启停和删除，`http_host` 会展示对应域名。</p>
      </div>

      <table>
        <thead>
          <tr>
            <th>名称</th>
            <th>类型</th>
            <th>本地目标</th>
            <th>域名/端口</th>
            <th>状态</th>
            <th>动作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="tunnel in tunnels" :key="tunnel.id">
            <td>{{ tunnel.name }}</td>
            <td>{{ tunnel.type }}</td>
            <td>{{ tunnel.local_host }}:{{ tunnel.local_port }}</td>
            <td>
              <template v-if="tunnel.type === 'tcp'">
                {{ tunnel.remote_port }}
              </template>
              <template v-else>
                <div class="route-stack">
                  <span v-for="route in routes[tunnel.id] || []" :key="route.id" class="route-pill">
                    {{ route.scheme }}://{{ route.domain }}
                    <button
                      class="route-delete"
                      :disabled="state.loading.value"
                      @click="state.removeDomainRoute(route)"
                    >
                      ×
                    </button>
                  </span>
                </div>
              </template>
            </td>
            <td>
              <span :class="['pill', tunnel.enabled ? 'online' : 'offline']">{{ tunnel.status }}</span>
            </td>
            <td class="action-row">
              <button class="ghost" :disabled="state.loading.value" @click="state.toggleTunnel(tunnel)">
                {{ tunnel.enabled ? "停用" : "启用" }}
              </button>
              <button class="ghost danger" :disabled="state.loading.value" @click="state.removeTunnel(tunnel)">
                删除
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>
