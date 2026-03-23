<script setup lang="ts">
import { computed, onMounted } from "vue";
import { useWorkspaceState } from "../state";

const state = useWorkspaceState();
const summary = computed(() => state.summary.value);
const lastSettlement = computed(() => state.lastSettlement.value);
const balanceText = computed(() => {
  if (!summary.value) {
    return "--";
  }
  return `${summary.value.account.balance} ${summary.value.account.currency}`;
});

onMounted(() => {
  if (!summary.value) {
    void state.loadAll();
  }
});
</script>

<template>
  <section class="panel">
    <div class="panel-head">
      <h2>账户与账单</h2>
      <p>后续迁到 Tauri 时，这里可以直接扩成充值订单和账单明细页。</p>
    </div>

    <div class="billing-layout">
      <div class="balance-chip" v-if="summary">
        <span>余额</span>
        <strong>{{ balanceText }}</strong>
        <em v-if="summary.disabled_billing_tunnels > 0">
          欠费停用 tunnel: {{ summary.disabled_billing_tunnels }}
        </em>
      </div>

      <form class="recharge-form" @submit.prevent="state.submitRecharge">
        <label>
          <span>充值金额</span>
          <input v-model="state.rechargeAmount.value" />
        </label>
        <label>
          <span>备注</span>
          <input v-model="state.rechargeRemark.value" />
        </label>
        <button class="accent" :disabled="state.loading.value">手工充值</button>
        <button class="ghost" type="button" :disabled="state.loading.value" @click="state.runSettlement">
          手动结算
        </button>
      </form>
    </div>

    <div v-if="lastSettlement" class="settlement-box">
      <strong>最近一次手动结算</strong>
      <span>charged_bytes={{ lastSettlement.chargedBytes }}</span>
      <span>charge_amount={{ lastSettlement.chargeAmount }}</span>
      <span>transaction_id={{ lastSettlement.transactionId || "-" }}</span>
    </div>

    <div class="table-list">
      <div class="list-head">
        <h3>最近账单</h3>
      </div>
      <table v-if="summary">
        <thead>
          <tr>
            <th>类型</th>
            <th>金额</th>
            <th>余额变化</th>
            <th>备注</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in summary.recent_transactions" :key="item.id">
            <td>{{ item.type }}</td>
            <td>{{ item.amount }}</td>
            <td>{{ item.balance_before }} -> {{ item.balance_after }}</td>
            <td>{{ item.remark || "-" }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
