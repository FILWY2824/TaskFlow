<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { telegram as tgApi, ApiError } from '@/api'
import type { TelegramBindToken, TelegramBinding } from '@/types'
import { fmtDateTime } from '@/utils'

const bindings = ref<TelegramBinding[]>([])
const errMsg = ref('')
const ok = ref('')

const loading = ref(false)
const tokenInfo = ref<TelegramBindToken | null>(null)
const pollHandle = ref<number | null>(null)
const pollExpiresAt = ref<Date | null>(null)
const remainingSec = ref(0)
const expireTickHandle = ref<number | null>(null)

async function load() {
  errMsg.value = ''
  try {
    bindings.value = await tgApi.listBindings()
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

onMounted(load)
onBeforeUnmount(() => {
  if (pollHandle.value) window.clearInterval(pollHandle.value)
  if (expireTickHandle.value) window.clearInterval(expireTickHandle.value)
})

async function startBind() {
  errMsg.value = ''
  ok.value = ''
  loading.value = true
  try {
    const t = await tgApi.createBindToken()
    tokenInfo.value = t
    pollExpiresAt.value = new Date(t.expires_at)
    remainingSec.value = Math.max(
      0,
      Math.floor((pollExpiresAt.value.getTime() - Date.now()) / 1000),
    )
    if (expireTickHandle.value) window.clearInterval(expireTickHandle.value)
    expireTickHandle.value = window.setInterval(() => {
      remainingSec.value = Math.max(
        0,
        Math.floor(((pollExpiresAt.value?.getTime() || 0) - Date.now()) / 1000),
      )
      if (remainingSec.value <= 0) {
        stopPolling()
      }
    }, 1000)
    if (pollHandle.value) window.clearInterval(pollHandle.value)
    pollHandle.value = window.setInterval(pollStatus, 3000)
    // 自动打开 deep link
    window.open(t.deep_link_web, '_blank', 'noopener,noreferrer')
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  } finally {
    loading.value = false
  }
}

function stopPolling() {
  if (pollHandle.value) {
    window.clearInterval(pollHandle.value)
    pollHandle.value = null
  }
  if (expireTickHandle.value) {
    window.clearInterval(expireTickHandle.value)
    expireTickHandle.value = null
  }
}

async function pollStatus() {
  if (!tokenInfo.value) return
  try {
    const s = await tgApi.bindStatus(tokenInfo.value.token)
    if (s.status === 'bound') {
      stopPolling()
      tokenInfo.value = null
      ok.value = '绑定成功 ✓'
      await load()
    } else if (s.status === 'expired') {
      stopPolling()
      tokenInfo.value = null
      errMsg.value = '绑定链接已过期,请重新生成'
    }
  } catch (e) {
    // 轮询失败一次不致命
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

async function unbind(id: number) {
  if (!confirm('确认解绑这个 Telegram 账号?')) return
  try {
    await tgApi.unbind(id)
    await load()
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

async function sendTest(id: number) {
  errMsg.value = ''
  ok.value = ''
  try {
    await tgApi.sendTest(id)
    ok.value = '测试消息已发送 ✓'
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

function copy(text: string) {
  navigator.clipboard?.writeText(text).then(
    () => (ok.value = '已复制到剪贴板'),
    () => (errMsg.value = '复制失败,请手动选中复制'),
  )
}
</script>

<template>
  <div>
    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>
    <div v-if="ok" class="success-text" style="margin-bottom: 8px">{{ ok }}</div>

    <div class="section-card">
      <h3>当前绑定</h3>
      <div v-if="bindings.length === 0" class="muted">尚未绑定 Telegram。绑定后,提醒到点会推送给你。</div>
      <div v-for="b in bindings" :key="b.id" class="kv">
        <div class="k">@{{ b.username || '(无用户名)' }}</div>
        <div class="v">
          chat_id: {{ b.chat_id }}<br />
          <span class="muted" style="font-size: 12px">绑定于 {{ fmtDateTime(b.created_at) }}</span>
        </div>
        <div>
          <button class="btn-secondary" @click="sendTest(b.id)">测试推送</button>
          <button class="btn-ghost btn-danger" @click="unbind(b.id)">解绑</button>
        </div>
      </div>
    </div>

    <div class="section-card">
      <h3>新增绑定</h3>
      <p class="muted" style="font-size: 13px">
        点击按钮后会生成一次性 deep link 并在新窗口打开 Telegram。在 Telegram 里点 START 后,本页会自动检测绑定状态。
      </p>
      <p class="muted" style="font-size: 13px">
        ⚠ 出于安全考虑,严禁在前端输入 Telegram 手机号 / 密码 / 验证码 / chat_id 来"绑定"。
      </p>
      <button class="btn-primary" :disabled="loading" @click="startBind">
        {{ loading ? '生成中…' : '开始绑定' }}
      </button>

      <div v-if="tokenInfo" style="margin-top: 14px">
        <div class="kv">
          <div class="k">机器人</div>
          <div class="v">@{{ tokenInfo.bot_username }}</div>
        </div>
        <div class="kv">
          <div class="k">Web 链接</div>
          <div class="v">
            <a :href="tokenInfo.deep_link_web" target="_blank" rel="noopener">{{ tokenInfo.deep_link_web }}</a>
            <button class="btn-ghost" @click="copy(tokenInfo.deep_link_web)">复制</button>
          </div>
        </div>
        <div class="kv">
          <div class="k">App 链接</div>
          <div class="v">
            {{ tokenInfo.deep_link_app }}
            <button class="btn-ghost" @click="copy(tokenInfo.deep_link_app)">复制</button>
          </div>
        </div>
        <div class="kv">
          <div class="k">剩余时间</div>
          <div class="v">{{ remainingSec }} 秒</div>
        </div>
        <div class="muted" style="margin-top: 6px; font-size: 12px">本页正在轮询服务端…</div>
      </div>
    </div>
  </div>
</template>
