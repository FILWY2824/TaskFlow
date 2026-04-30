<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
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
    remainingSec.value = Math.max(0, Math.floor((pollExpiresAt.value.getTime() - Date.now()) / 1000))
    if (expireTickHandle.value) window.clearInterval(expireTickHandle.value)
    expireTickHandle.value = window.setInterval(() => {
      remainingSec.value = Math.max(0, Math.floor(((pollExpiresAt.value?.getTime() || 0) - Date.now()) / 1000))
      if (remainingSec.value <= 0) stopPolling()
    }, 1000)
    if (pollHandle.value) window.clearInterval(pollHandle.value)
    pollHandle.value = window.setInterval(pollStatus, 3000)
    // 不强行 window.open——某些浏览器会拦截弹窗。让用户主动点链接。
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  } finally {
    loading.value = false
  }
}

function stopPolling() {
  if (pollHandle.value) { window.clearInterval(pollHandle.value); pollHandle.value = null }
  if (expireTickHandle.value) { window.clearInterval(expireTickHandle.value); expireTickHandle.value = null }
}

async function pollStatus() {
  if (!tokenInfo.value) return
  try {
    const s = await tgApi.bindStatus(tokenInfo.value.token)
    if (s.status === 'bound') {
      stopPolling()
      tokenInfo.value = null
      ok.value = '✓ 绑定成功！今后到点的提醒会推送到 Telegram。'
      await load()
    } else if (s.status === 'expired') {
      stopPolling()
      tokenInfo.value = null
      errMsg.value = '绑定链接已过期，请重新生成。'
    }
    // pending / not_found 继续等
  } catch (e) {
    // 单次轮询失败不要打断；只记最后一次错
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

async function checkNow() {
  await pollStatus()
}

async function unbind(id: number) {
  if (!confirm('确认解绑这个 Telegram 账号？')) return
  try {
    await tgApi.unbind(id)
    await load()
    ok.value = '已解绑'
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

async function sendTest(id: number) {
  errMsg.value = ''
  ok.value = ''
  try {
    await tgApi.sendTest(id)
    ok.value = '✓ 测试消息已发送'
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

function copy(text: string) {
  navigator.clipboard?.writeText(text).then(
    () => (ok.value = '已复制到剪贴板'),
    () => (errMsg.value = '复制失败，请手动选中复制'),
  )
}

const remainingLabel = computed(() => {
  const s = remainingSec.value
  if (s <= 0) return '已过期'
  const m = Math.floor(s / 60)
  const sec = s % 60
  return `${m} 分 ${String(sec).padStart(2, '0')} 秒`
})
</script>

<template>
  <div class="tg-page">
    <Transition name="fade">
      <div v-if="ok" class="banner banner-ok">{{ ok }}</div>
    </Transition>
    <Transition name="fade">
      <div v-if="errMsg" class="banner banner-err">{{ errMsg }}</div>
    </Transition>

    <div class="settings-card">
      <div class="card-head">
        <h3>当前绑定</h3>
        <p class="card-hint">绑定 Telegram 后，到点的提醒会推送到对应聊天。</p>
      </div>
      <div class="card-body">
        <div v-if="bindings.length === 0" class="empty-mini">尚未绑定 Telegram。</div>
        <div v-for="b in bindings" :key="b.id" class="binding-item">
          <div class="binding-icon">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/>
            </svg>
          </div>
          <div class="binding-info">
            <div class="binding-name">@{{ b.username || '(无用户名)' }}</div>
            <div class="binding-meta">chat_id: {{ b.chat_id }} · 绑定于 {{ fmtDateTime(b.created_at) }}</div>
          </div>
          <div class="binding-actions">
            <button class="btn-secondary btn-sm" @click="sendTest(b.id)">测试推送</button>
            <button class="btn-ghost btn-danger btn-sm" @click="unbind(b.id)">解绑</button>
          </div>
        </div>
      </div>
    </div>

    <div class="settings-card">
      <div class="card-head">
        <h3>新增绑定</h3>
        <p class="card-hint">
          点击「生成绑定链接」后，打开 Telegram 链接、按下机器人对话窗口里的 <strong>START</strong>，本页会自动检测绑定状态。
        </p>
      </div>
      <div class="card-body">
        <div class="warn-strip">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
            <line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/>
          </svg>
          <span>出于安全考虑，严禁在前端输入 Telegram 手机号 / 密码 / 验证码 / chat_id 来"绑定"。请走机器人 deep link 流程。</span>
        </div>

        <div v-if="!tokenInfo" class="action-row">
          <button class="btn-primary" :disabled="loading" @click="startBind">
            {{ loading ? '生成中…' : '生成绑定链接' }}
          </button>
        </div>

        <div v-else class="bind-panel">
          <div class="form-row">
            <label>机器人</label>
            <div class="form-static">@{{ tokenInfo.bot_username }}</div>
          </div>
          <div class="form-row">
            <label>剩余时间</label>
            <div class="form-static" :class="{ 'danger-text': remainingSec < 60 }">{{ remainingLabel }}</div>
          </div>
          <div class="form-row">
            <label>Web 链接</label>
            <div class="form-input-wrap">
              <a class="link-input" :href="tokenInfo.deep_link_web" target="_blank" rel="noopener">{{ tokenInfo.deep_link_web }}</a>
              <button class="btn-ghost btn-sm" @click="copy(tokenInfo.deep_link_web)">复制</button>
            </div>
          </div>
          <div class="form-row">
            <label>App 链接</label>
            <div class="form-input-wrap">
              <span class="link-input">{{ tokenInfo.deep_link_app }}</span>
              <button class="btn-ghost btn-sm" @click="copy(tokenInfo.deep_link_app)">复制</button>
            </div>
          </div>
          <div class="poll-tip">
            <span class="dot-pulse" />
            正在等待 Telegram 端确认…
            <button class="btn-ghost btn-sm" style="margin-left:8px" @click="checkNow">立即检查</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tg-page { max-width: 760px; margin: 0 auto; display: flex; flex-direction: column; gap: 18px; }

.banner { padding: 10px 14px; border-radius: var(--tg-radius-md); font-size: 13.5px; font-weight: 500; }
.banner-ok { background: var(--tg-success-soft); color: var(--tg-success); }
.banner-err { background: var(--tg-danger-soft); color: var(--tg-danger); }

.settings-card {
  background: var(--tg-side);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-lg);
  overflow: hidden;
  box-shadow: var(--tg-shadow-sm);
}
.card-head { padding: 16px 20px 12px; border-bottom: 1px solid var(--tg-divider); }
.card-head h3 { margin: 0; font-size: 15px; font-weight: 700; letter-spacing: -0.2px; }
.card-head .card-hint { margin: 6px 0 0; font-size: 12.5px; color: var(--tg-text-secondary); line-height: 1.55; }
.card-body { padding: 14px 20px; }

.empty-mini { color: var(--tg-text-secondary); font-size: 13.5px; padding: 6px 0; }

.binding-item {
  display: flex; align-items: center; gap: 12px;
  padding: 10px 0;
  border-bottom: 1px solid var(--tg-divider);
}
.binding-item:last-child { border-bottom: none; }
.binding-icon {
  width: 36px; height: 36px;
  background: var(--tg-primary-soft); color: var(--tg-primary);
  border-radius: 50%;
  display: flex; align-items: center; justify-content: center; flex-shrink: 0;
}
.binding-info { flex: 1; min-width: 0; }
.binding-name { font-weight: 600; font-size: 14px; }
.binding-meta { font-size: 12px; color: var(--tg-text-secondary); margin-top: 2px; }
.binding-actions { display: flex; gap: 6px; flex-shrink: 0; }
.btn-sm { padding: 6px 12px !important; font-size: 12.5px !important; }

.warn-strip {
  display: flex; align-items: flex-start; gap: 8px;
  padding: 10px 12px;
  background: var(--tg-warn-soft);
  color: var(--tg-warn);
  border-radius: var(--tg-radius-sm);
  font-size: 12.5px; line-height: 1.55;
  margin-bottom: 14px;
}
.warn-strip svg { flex-shrink: 0; margin-top: 1px; }

.action-row { display: flex; gap: 10px; }

.bind-panel {
  display: flex; flex-direction: column; gap: 10px;
  padding: 4px 0;
}
.form-row {
  display: grid;
  grid-template-columns: 90px 1fr;
  align-items: center;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px solid var(--tg-divider);
}
.form-row:last-of-type { border-bottom: none; }
.form-row label { font-size: 12.5px; color: var(--tg-text-secondary); font-weight: 500; }
.form-static { font-size: 13.5px; }
.form-input-wrap { display: flex; gap: 8px; align-items: center; min-width: 0; }
.link-input {
  flex: 1; min-width: 0;
  padding: 7px 10px;
  background: var(--tg-hover);
  border-radius: 6px;
  font-size: 12px;
  font-family: ui-monospace, SFMono-Regular, monospace;
  color: var(--tg-text);
  text-decoration: none;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
a.link-input:hover { background: var(--tg-press); color: var(--tg-primary); }

.poll-tip {
  display: flex; align-items: center;
  margin-top: 6px;
  padding: 10px 12px;
  background: var(--tg-primary-soft);
  color: var(--tg-primary);
  border-radius: var(--tg-radius-sm);
  font-size: 13px;
}
.dot-pulse {
  display: inline-block;
  width: 8px; height: 8px;
  background: var(--tg-primary);
  border-radius: 50%;
  margin-right: 8px;
  animation: pulse 1.4s ease-in-out infinite;
}
@keyframes pulse {
  0%, 100% { opacity: 0.3; transform: scale(0.85); }
  50% { opacity: 1; transform: scale(1.1); }
}

@media (max-width: 600px) {
  .form-row { grid-template-columns: 1fr; gap: 4px; }
  .binding-item { flex-wrap: wrap; }
  .binding-actions { width: 100%; justify-content: flex-end; }
}
</style>
