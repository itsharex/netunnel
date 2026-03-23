<script setup lang="ts">
import { computed, onMounted } from "vue";
import { useWorkspaceState } from "../state";

const state = useWorkspaceState();
const tunnels = computed(() => state.tunnels.value);
const usageConnections = computed(() => state.usageConnections.value);
const usageTraffic = computed(() => state.usageTraffic.value);

onMounted(async () => {
  if (tunnels.value.length === 0) {
    await state.loadAll();
  } else {
    await state.reloadUsage();
  }
});
</script>

<template>
  <div class="page-stack">
    <section class="panel">
      <div class="panel-head">
        <h2>流量与连接筛选</h2>
        <p>这里的数据直接来自 `usage/connections` 和 `usage/traffic` 接口。</p>
      </div>

      <form class="inline-form three-columns" @submit.prevent="state.reloadUsage">
        <label>
          <span>Tunnel</span>
          <select v-model="state.usageFilter.value.tunnelId">
            <option value="">全部 tunnel</option>
            <option v-for="tunnel in tunnels" :key="tunnel.id" :value="tunnel.id">
              {{ tunnel.name }}
            </option>
          </select>
        </label>
        <label>
          <span>连接条数</span>
          <input v-model="state.usageFilter.value.limit" />
        </label>
        <label>
          <span>统计小时数</span>
          <input v-model="state.usageFilter.value.hours" />
        </label>
        <div class="form-action-cell">
          <button class="accent" :disabled="state.loading.value">刷新使用量</button>
        </div>
      </form>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>最近连接</h2>
        <p>适合定位哪个 tunnel 在被访问、走了多少流量。</p>
      </div>

      <table>
        <thead>
          <tr>
            <th>协议</th>
            <th>Tunnel</th>
            <th>来源</th>
            <th>目标</th>
            <th>流量</th>
            <th>开始时间</th>
            <th>状态</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in usageConnections" :key="item.id">
            <td>{{ item.protocol }}</td>
            <td>{{ item.tunnel_id }}</td>
            <td>{{ item.source_addr || "-" }}</td>
            <td>{{ item.target_addr || "-" }}</td>
            <td>{{ item.total_bytes }} bytes</td>
            <td>{{ item.started_at }}</td>
            <td><span class="pill online">{{ item.status }}</span></td>
          </tr>
        </tbody>
      </table>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>小时流量桶</h2>
        <p>后续做桌面图表时，可以直接从这组数据驱动。</p>
      </div>

      <div class="usage-list">
        <article v-for="item in usageTraffic" :key="item.id" class="usage-item">
          <strong>{{ item.tunnel_id || "all" }}</strong>
          <span>bucket={{ item.bucket_time }}</span>
          <span>ingress={{ item.ingress_bytes }} bytes</span>
          <span>egress={{ item.egress_bytes }} bytes</span>
          <span>total={{ item.total_bytes }} bytes</span>
          <span>billed={{ item.billed_bytes }} bytes</span>
        </article>
      </div>
    </section>
  </div>
</template>
