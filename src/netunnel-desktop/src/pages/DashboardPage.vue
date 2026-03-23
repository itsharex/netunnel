<script setup lang="ts">
import { computed, onMounted } from "vue";
import { useWorkspaceState } from "../state";

const state = useWorkspaceState();

const summary = computed(() => state.summary.value);
const cards = computed(() => state.cards.value);

onMounted(() => {
  if (!summary.value) {
    void state.loadAll();
  }
});
</script>

<template>
  <div class="page-stack">
    <section class="card-grid">
      <article v-for="card in cards" :key="card.label" class="metric-card">
        <span>{{ card.label }}</span>
        <strong>{{ card.value }}</strong>
      </article>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>系统概览</h2>
        <p>用于桌面首页快速判断账户、节点和隧道健康度。</p>
      </div>

      <div class="spotlight-grid" v-if="summary">
        <article class="spotlight-card warm">
          <span>欠费停用 Tunnel</span>
          <strong>{{ summary.disabled_billing_tunnels }}</strong>
          <p>充值后会自动恢复系统因欠费禁用的通道。</p>
        </article>
        <article class="spotlight-card cool">
          <span>未结算 24h 流量</span>
          <strong>{{ summary.unbilled_traffic_bytes_24h }} bytes</strong>
          <p>当前账务后台会按周期自动推进 billed_bytes。</p>
        </article>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>最近流量</h2>
        <p>桌面端后续可以直接把这里换成图表。</p>
      </div>
      <div class="usage-list" v-if="summary">
        <article v-for="usage in summary.recent_usages" :key="usage.id" class="usage-item">
          <strong>{{ usage.tunnel_id }}</strong>
          <span>total={{ usage.total_bytes }} bytes</span>
          <span>billed={{ usage.billed_bytes }} bytes</span>
          <span>bucket={{ usage.bucket_time }}</span>
        </article>
      </div>
    </section>
  </div>
</template>
