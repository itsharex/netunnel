<script setup lang="ts">
import { computed } from "vue";
import { useWorkspaceState } from "../state";

const state = useWorkspaceState();
const sessionSummary = computed(() => state.sessionSummary.value);
</script>

<template>
  <div class="page-stack">
    <section class="panel">
      <div class="panel-head">
        <h2>会话概览</h2>
        <p>当前还是开发期模式，但这里已经预留了未来正式登录和 token 接入的位置。</p>
      </div>

      <div class="spotlight-grid">
        <article class="spotlight-card warm">
          <span>会话模式</span>
          <strong>{{ sessionSummary.mode }}</strong>
          <p>当前管理 API 仍使用显式 `user_id`，后续可切到 bearer token。</p>
        </article>
        <article class="spotlight-card cool">
          <span>Token 状态</span>
          <strong>{{ sessionSummary.hasToken ? "已填写" : "未填写" }}</strong>
          <p>前端已预留 token 输入和持久化，后端接鉴权后可以直接切换。</p>
        </article>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>当前会话</h2>
        <p>这部分是未来 Tauri 设置页的雏形。</p>
      </div>

      <form class="inline-form two-columns">
        <label>
          <span>Session Mode</span>
          <select v-model="state.sessionMode.value">
            <option value="development">development</option>
            <option value="token-ready">token-ready</option>
          </select>
        </label>
        <label>
          <span>User ID</span>
          <input v-model="state.userId.value" />
        </label>
        <label class="full-span">
          <span>Access Token</span>
          <input v-model="state.accessToken.value" placeholder="未来接正式登录后使用" />
        </label>
      </form>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>开发期用户创建</h2>
        <p>当前直接调用 `/api/v1/dev/bootstrap-user`，用于快速生成新的测试用户并切换会话。</p>
      </div>

      <form class="inline-form two-columns" @submit.prevent="state.bootstrapDevelopmentUser">
        <label>
          <span>Email</span>
          <input v-model="state.bootstrapForm.value.email" />
        </label>
        <label>
          <span>Nickname</span>
          <input v-model="state.bootstrapForm.value.nickname" />
        </label>
        <label class="full-span">
          <span>Password</span>
          <input v-model="state.bootstrapForm.value.password" type="password" />
        </label>
        <div class="form-action-cell">
          <button class="accent" :disabled="state.loading.value">创建并切换开发用户</button>
        </div>
      </form>
    </section>
  </div>
</template>
