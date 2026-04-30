<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { telegram as tgApi, ApiError } from '@/api'
import type { TelegramBindToken, TelegramBinding, TelegramConfig } from '@/types'
import { fmtDateTime } from '@/utils'
import { confirmDialog } from '@/dialogs'

const bindings = ref<TelegramBinding[]>([])
const errMsg = ref('')
const ok = ref('')

const cfg = ref<TelegramConfig | null>(null)
const cfgLoading = ref(true)

const loading = ref(false)
const tokenInfo = ref<TelegramBindToken | null>(null)
const pollHandle = ref<number | null>(null)
const pollExpiresAt = ref<Date | null>(null)
const remainingSec = ref(0)
const expireTickHandle = ref<number | null>(null)

const showHelp = ref(false)

onMounted(async () => {
  await Promise.all([loadConfig(), loadBindings()])
})

onBeforeUnmount(() => {
  if (pollHandle.value) window.clearInterval(pollHandle.value)
  if (expireTickHandle.value) window.clearInterval(expireTickHandle.value)
})

async function loadConfig() {
  cfgLoading.value = true
  try {
    cfg.value = await tgApi.getConfig()
  } catch {
    cfg.value = { enabled: false, bot_username: '' }
  } finally {
    cfgLoading.value = false
  }
}

async function loadBindings() {
  errMsg.value = ''
  try {
    bindings.value = await tgApi.listBindings()
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

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
      ok.value = '🎉 绑定成功！今后到点的提醒会推送到 Telegram。'
      await loadBindings()
    } else if (s.status === 'expired') {
      stopPolling()
      tokenInfo.value = null
      errMsg.value = '绑定链接已过期，请重新生成。'
    }
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

async function checkNow() { await pollStatus() }

function cancelBind() {
  stopPolling()
  tokenInfo.value = null
}

async function unbind(id: number) {
  const yes = await confirmDialog({
    title: '确认解绑这个 Telegram 账号？',
    message: '解绑后将不再向该账号推送提醒。如需重新绑定，可以再次走绑定流程。',
    confirmText: '解绑',
    cancelText: '取消',
    danger: true,
  })
  if (!yes) return
  try {
    await tgApi.unbind(id)
    await loadBindings()
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
    ok.value = '✅ 测试消息已发送，请查看 Telegram'
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

const botHandle = computed(() => {
  if (tokenInfo.value?.bot_username) return '@' + tokenInfo.value.bot_username
  if (cfg.value?.bot_username) return '@' + cfg.value.bot_username
  return ''
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

    <!-- Hero -->
    <div class="hero">
      <div class="hero-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/>
        </svg>
      </div>
      <div class="hero-text">
        <h2>把任务提醒推送到 Telegram</h2>
        <p>到时间没看 App 也不会错过——绑定后，所有提醒会同时推到你的 Telegram 聊天。</p>
      </div>
      <button class="btn-help" @click="showHelp = true" aria-label="如何绑定？">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="10"/>
          <path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/>
          <line x1="12" y1="17" x2="12.01" y2="17"/>
        </svg>
        如何绑定？
      </button>
    </div>

    <!-- 服务端未启用 -->
    <div v-if="!cfgLoading && cfg && !cfg.enabled" class="cfg-disabled">
      <div class="cfg-disabled-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="10"/><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/>
        </svg>
      </div>
      <div class="cfg-disabled-info">
        <h3>管理员尚未启用 Telegram 集成</h3>
        <p>
          这是一个服务端配置项。运维同学需要在 <code>server/config.toml</code> 的
          <code>[telegram]</code> 段中填入 <code>bot_token</code>、<code>bot_username</code>、
          <code>webhook_secret</code>，然后重启服务即可启用。
        </p>
        <p class="muted" style="margin-top: 6px;">
          BotFather 是创建 bot 的官方途径，详见
          <a href="https://core.telegram.org/bots/features#botfather" target="_blank" rel="noopener">core.telegram.org/bots</a>。
        </p>
      </div>
    </div>

    <!-- 当前绑定 -->
    <div v-if="!cfgLoading && cfg && cfg.enabled" class="settings-card">
      <div class="card-head">
        <h3>当前绑定</h3>
        <p class="card-hint">绑定后，到点的提醒会推送到对应聊天。可以同时绑定多个聊天。</p>
      </div>
      <div class="card-body">
        <div v-if="bindings.length === 0" class="empty-mini">
          <span class="empty-mini-dot" />
          尚未绑定 Telegram。请按下方步骤完成绑定 ↓
        </div>
        <div v-for="b in bindings" :key="b.id" class="binding-item">
          <div class="binding-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/>
            </svg>
          </div>
          <div class="binding-info">
            <div class="binding-name">@{{ b.username || '(无用户名)' }}</div>
            <div class="binding-meta">chat_id: <code>{{ b.chat_id }}</code> · 绑定于 {{ fmtDateTime(b.created_at) }}</div>
          </div>
          <div class="binding-actions">
            <button class="btn-secondary btn-sm" @click="sendTest(b.id)">测试推送</button>
            <button class="btn-ghost btn-danger btn-sm" @click="unbind(b.id)">解绑</button>
          </div>
        </div>
      </div>
    </div>

    <!-- 步骤化绑定 -->
    <div v-if="!cfgLoading && cfg && cfg.enabled" class="settings-card bind-card">
      <div class="card-head">
        <h3>新增绑定</h3>
        <p class="card-hint">三步搞定。出于安全考虑，绑定走 Telegram 官方 deep link，不需要你输入手机号或验证码。</p>
      </div>
      <div class="card-body">
        <ol class="steps">
          <li class="step" :class="{ 'is-current': !tokenInfo, 'is-done': !!tokenInfo }">
            <span class="step-num">1</span>
            <div class="step-body">
              <div class="step-title">点下方按钮生成绑定链接</div>
              <div class="step-desc">服务端会生成一个一次性 token（10 分钟有效）。</div>
            </div>
          </li>
          <li class="step" :class="{ 'is-current': !!tokenInfo }">
            <span class="step-num">2</span>
            <div class="step-body">
              <div class="step-title">打开 Telegram，按 <strong>START / 开始</strong></div>
              <div class="step-desc">链接会跳到 {{ botHandle || '机器人' }} 对话；按 START 即可。</div>
            </div>
          </li>
          <li class="step">
            <span class="step-num">3</span>
            <div class="step-body">
              <div class="step-title">本页自动检测 → 完成</div>
              <div class="step-desc">无需手动刷新，绑定成功后顶上会出现 ✅ 提示。</div>
            </div>
          </li>
        </ol>

        <div class="warn-strip">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
            <line x1="12" y1="9" x2="12" y2="13"/>
            <line x1="12" y1="17" x2="12.01" y2="17"/>
          </svg>
          <span>不要在任何地方输入 Telegram 手机号 / 密码 / 验证码 / chat_id 来"绑定"——本应用只通过官方 deep link 完成。</span>
        </div>

        <div v-if="!tokenInfo" class="action-row">
          <button class="btn-primary big-cta" :disabled="loading" @click="startBind">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
              <line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/>
            </svg>
            {{ loading ? '生成中…' : '生成绑定链接' }}
          </button>
        </div>

        <div v-else class="bind-panel">
          <div class="bind-status">
            <span class="dot-pulse" />
            <div class="bind-status-text">
              <div class="bind-status-title">正在等待 Telegram 端确认…</div>
              <div class="bind-status-meta">
                机器人 <strong>{{ botHandle }}</strong> · 剩余
                <strong :class="{ 'danger-text': remainingSec < 60 }">{{ remainingLabel }}</strong>
              </div>
            </div>
            <button class="btn-ghost" @click="checkNow">立即检查</button>
            <button class="btn-ghost btn-danger" @click="cancelBind">取消</button>
          </div>

          <div class="link-grid">
            <a class="link-card primary" :href="tokenInfo.deep_link_web" target="_blank" rel="noopener">
              <div class="link-card-head">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>
                  <polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/>
                </svg>
                <span>在浏览器打开 Telegram</span>
                <span class="link-card-tag">推荐</span>
              </div>
              <div class="link-card-body">{{ tokenInfo.deep_link_web }}</div>
              <div class="link-card-foot">
                <span>点此卡片直接打开 →</span>
                <button class="btn-ghost btn-sm" @click.prevent="copy(tokenInfo.deep_link_web)">复制</button>
              </div>
            </a>

            <div class="link-card">
              <div class="link-card-head">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                  <rect x="5" y="2" width="14" height="20" rx="2"/><line x1="12" y1="18" x2="12.01" y2="18"/>
                </svg>
                <span>在桌面 / 手机 App 打开</span>
              </div>
              <div class="link-card-body">{{ tokenInfo.deep_link_app }}</div>
              <div class="link-card-foot">
                <a class="btn-ghost btn-sm" :href="tokenInfo.deep_link_app">尝试打开 App</a>
                <button class="btn-ghost btn-sm" @click="copy(tokenInfo.deep_link_app)">复制</button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 帮助 Modal -->
    <Transition name="fade">
      <div v-if="showHelp" class="modal-backdrop" @click.self="showHelp = false">
        <div class="modal-card help-modal">
          <header class="modal-head">
            <span class="modal-title">如何绑定 Telegram？</span>
            <button class="btn-icon" @click="showHelp = false" aria-label="关闭">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </header>
          <div class="modal-body">
            <p class="help-lead">
              简单来说：<strong>点一个按钮 → Telegram 弹出对话 → 按 START</strong>。
              整个过程不需要输任何隐私信息。
            </p>

            <div class="help-step">
              <div class="help-step-num">1</div>
              <div>
                <h4>点击「生成绑定链接」</h4>
                <p>本页会向后端要一个一次性 token（10 分钟有效），并拼出两条链接：网页版的 <code>t.me/...</code> 和 App 版的 <code>tg://...</code>。</p>
              </div>
            </div>

            <div class="help-step">
              <div class="help-step-num">2</div>
              <div>
                <h4>打开链接，进 Telegram 机器人对话</h4>
                <p>
                  推荐点"在浏览器打开"那张卡——会跳到 Telegram Web，登录后自动进入机器人对话。
                  电脑/手机已装 Telegram App 的，也可以点"App 链接"直接唤起本地客户端。
                </p>
              </div>
            </div>

            <div class="help-step">
              <div class="help-step-num">3</div>
              <div>
                <h4>按 Telegram 里的 <kbd>START</kbd> 按钮</h4>
                <p>
                  Telegram 进入新对话时底部一定会出现 <kbd>START</kbd>（中文版叫 <kbd>开始</kbd>）。
                  按下去，机器人会回一句"绑定成功"。
                </p>
                <p class="muted">机器人会带着你的 token 进入服务端流程，把这个聊天的 chat_id 记到你的账号下——之后所有提醒就会发到这里。</p>
              </div>
            </div>

            <div class="help-step">
              <div class="help-step-num">4</div>
              <div>
                <h4>回到本页 → 自动出现 ✅ 已绑定</h4>
                <p>本页每 3 秒检测一次状态。如果你嫌慢，按"立即检查"也行。</p>
              </div>
            </div>

            <hr />

            <h4 class="trouble-h">如果绑定失败？</h4>
            <ul class="trouble-list">
              <li><strong>「管理员尚未启用」</strong>——后端 <code>config.toml</code> 没填 bot_token / bot_username / webhook_secret。让管理员补上后重启服务。</li>
              <li><strong>链接已过期</strong>——token 10 分钟有效。点"取消"，重新生成即可。</li>
              <li><strong>按了 START 但页面一直转</strong>——大概率是 webhook 没接通。让管理员检查：(1) 域名能从公网访问；(2) 部署完成后调用过 <code>setWebhook</code>（参考 <code>deploy/scripts/</code>）。</li>
              <li><strong>"该 Telegram 账号已绑定到另一用户"</strong>——同一个 chat 不能同时挂在两个账号下。在原账号里先解绑，再到这里重新绑。</li>
            </ul>
          </div>
          <footer class="modal-foot">
            <button class="btn-primary" @click="showHelp = false">明白了</button>
          </footer>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.tg-page {
  max-width: 820px;
  margin: 0 auto;
  display: flex; flex-direction: column;
  gap: 18px;
}

/* === Hero === */
.hero {
  display: flex; align-items: center; gap: 18px;
  padding: 24px 28px;
  background: linear-gradient(135deg,
    rgba(99, 102, 241, 0.10),
    rgba(14, 165, 233, 0.08) 50%,
    rgba(217, 70, 239, 0.08));
  border: 1px solid color-mix(in srgb, var(--tg-primary) 20%, transparent);
  border-radius: var(--tg-radius-xl);
  position: relative;
  overflow: hidden;
}
.hero::before {
  content: '';
  position: absolute; inset: -40%;
  background: radial-gradient(circle at 20% 30%, rgba(99,102,241,0.20), transparent 50%);
  pointer-events: none;
}
.hero-icon {
  width: 56px; height: 56px;
  display: flex; align-items: center; justify-content: center;
  background: linear-gradient(135deg, #229ED9, #0088cc);
  color: #fff;
  border-radius: var(--tg-radius-md);
  flex-shrink: 0;
  box-shadow: 0 8px 24px -4px rgba(34, 158, 217, 0.45);
  position: relative; z-index: 1;
}
.hero-icon svg { width: 28px; height: 28px; }
.hero-text { flex: 1; min-width: 0; position: relative; z-index: 1; }
.hero-text h2 {
  font-family: 'Sora', sans-serif;
  font-size: 19px; font-weight: 700;
  margin: 0 0 4px;
  letter-spacing: -0.018em;
}
.hero-text p {
  margin: 0;
  font-size: 13.5px; line-height: 1.55;
  color: var(--tg-text-secondary);
}
.btn-help {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 9px 16px;
  background: var(--tg-bg-elev);
  color: var(--tg-primary);
  font-weight: 600; font-size: 13px;
  border: 1.5px solid color-mix(in srgb, var(--tg-primary) 30%, transparent);
  border-radius: var(--tg-radius-pill);
  flex-shrink: 0;
  position: relative; z-index: 1;
  cursor: pointer;
  transition: all var(--tg-trans-fast);
}
.btn-help:hover {
  background: var(--tg-primary);
  color: var(--tg-on-primary);
  border-color: var(--tg-primary);
  transform: translateY(-1px);
  box-shadow: var(--tg-shadow-md);
}
.btn-help svg { width: 16px; height: 16px; }

/* === 未配置 banner === */
.cfg-disabled {
  display: flex; gap: 16px;
  padding: 18px 22px;
  background: var(--tg-warn-soft);
  border: 1px solid color-mix(in srgb, var(--tg-warn) 25%, transparent);
  border-radius: var(--tg-radius-lg);
}
.cfg-disabled-icon {
  width: 40px; height: 40px;
  display: flex; align-items: center; justify-content: center;
  background: var(--tg-warn);
  color: #fff;
  border-radius: 50%;
  flex-shrink: 0;
}
.cfg-disabled-icon svg { width: 20px; height: 20px; }
.cfg-disabled-info { flex: 1; min-width: 0; }
.cfg-disabled-info h3 {
  font-family: 'Sora', sans-serif;
  font-size: 15px; font-weight: 700;
  color: var(--tg-warn);
  margin: 0 0 6px;
}
.cfg-disabled-info p {
  margin: 0;
  font-size: 13px; line-height: 1.6;
  color: var(--tg-text);
}
.cfg-disabled-info code {
  background: rgba(0,0,0,0.06);
  color: var(--tg-text);
  font-size: 12px;
}

/* === 设置卡 === */
.settings-card {
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-lg);
  overflow: hidden;
  box-shadow: var(--tg-shadow-xs);
}
.card-head {
  padding: 18px 22px 12px;
  border-bottom: 1px solid var(--tg-divider);
}
.card-head h3 {
  font-family: 'Sora', sans-serif;
  margin: 0; font-size: 15.5px; font-weight: 700;
  letter-spacing: -0.012em;
}
.card-head .card-hint {
  margin: 6px 0 0;
  font-size: 13px; color: var(--tg-text-secondary);
  line-height: 1.55;
}
.card-body { padding: 16px 22px; }

.empty-mini {
  display: flex; align-items: center; gap: 10px;
  padding: 12px 14px;
  background: var(--tg-hover);
  border-radius: var(--tg-radius-sm);
  color: var(--tg-text-secondary);
  font-size: 13px;
}
.empty-mini-dot {
  width: 8px; height: 8px;
  border-radius: 50%;
  background: var(--tg-text-tertiary);
}

.binding-item {
  display: flex; align-items: center; gap: 14px;
  padding: 12px 0;
  border-bottom: 1px solid var(--tg-divider);
}
.binding-item:last-child { border-bottom: none; }
.binding-icon {
  width: 40px; height: 40px;
  display: flex; align-items: center; justify-content: center;
  background: linear-gradient(135deg, #229ED9, #0088cc);
  color: #fff;
  border-radius: 50%;
  flex-shrink: 0;
  box-shadow: 0 4px 12px -2px rgba(34, 158, 217, 0.40);
}
.binding-info { flex: 1; min-width: 0; }
.binding-name { font-weight: 700; font-size: 14.5px; }
.binding-meta {
  font-size: 12px; color: var(--tg-text-tertiary); margin-top: 2px;
}
.binding-meta code {
  background: var(--tg-hover);
  padding: 1px 5px; border-radius: 4px;
  font-size: 11.5px;
}
.binding-actions { display: flex; gap: 6px; flex-shrink: 0; }
.btn-sm { padding: 6px 12px !important; font-size: 12.5px !important; }

/* === 步骤 === */
.steps {
  list-style: none;
  padding: 0;
  margin: 0 0 16px;
  display: flex;
  gap: 10px;
}
.step {
  flex: 1; min-width: 0;
  display: flex; gap: 12px; align-items: flex-start;
  padding: 14px;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-md);
  transition: all var(--tg-trans);
  position: relative;
}
.step.is-current {
  border-color: var(--tg-primary);
  background: var(--tg-primary-soft);
  box-shadow: var(--tg-shadow-md);
}
.step.is-done .step-num {
  background: var(--tg-grad-brand);
  color: var(--tg-on-primary);
}
.step-num {
  width: 28px; height: 28px;
  display: flex; align-items: center; justify-content: center;
  flex-shrink: 0;
  background: var(--tg-hover);
  color: var(--tg-text-secondary);
  border-radius: 50%;
  font-family: 'Sora', sans-serif;
  font-weight: 800; font-size: 14px;
  transition: all var(--tg-trans);
}
.step.is-current .step-num {
  background: var(--tg-grad-brand);
  color: var(--tg-on-primary);
  transform: scale(1.06);
  box-shadow: var(--tg-shadow-glow);
}
.step-body { min-width: 0; }
.step-title {
  font-weight: 700; font-size: 13.5px;
  color: var(--tg-text);
  line-height: 1.4;
}
.step-title strong {
  background: var(--tg-grad-brand);
  -webkit-background-clip: text; background-clip: text;
  color: transparent;
  font-weight: 800;
}
.step-desc {
  margin-top: 4px;
  font-size: 12px;
  color: var(--tg-text-tertiary);
  line-height: 1.5;
}

@media (max-width: 700px) {
  .steps { flex-direction: column; }
}

/* === warn === */
.warn-strip {
  display: flex; align-items: flex-start; gap: 8px;
  padding: 10px 14px;
  background: var(--tg-warn-soft);
  color: var(--tg-warn);
  border-radius: var(--tg-radius-sm);
  font-size: 12.5px; line-height: 1.55;
  margin-bottom: 16px;
}
.warn-strip svg { flex-shrink: 0; margin-top: 1px; }

.action-row { display: flex; gap: 10px; }
.big-cta {
  padding: 14px 28px !important;
  font-size: 15px !important;
  font-weight: 700 !important;
  letter-spacing: -0.01em;
}

/* === bind panel (active state) === */
.bind-panel {
  display: flex; flex-direction: column; gap: 14px;
  padding: 4px 0;
}
.bind-status {
  display: flex; align-items: center; gap: 12px;
  padding: 14px 16px;
  background: var(--tg-grad-brand-soft);
  border: 1px solid color-mix(in srgb, var(--tg-primary) 28%, transparent);
  border-radius: var(--tg-radius-md);
}
.bind-status-text { flex: 1; min-width: 0; }
.bind-status-title {
  font-weight: 700; font-size: 13.5px;
  color: var(--tg-primary);
}
.bind-status-meta {
  margin-top: 2px;
  font-size: 12px;
  color: var(--tg-text-secondary);
}
.bind-status-meta strong { color: var(--tg-text); font-variant-numeric: tabular-nums; }
.danger-text { color: var(--tg-danger) !important; }

.dot-pulse {
  display: inline-block;
  width: 12px; height: 12px;
  background: var(--tg-primary);
  border-radius: 50%;
  flex-shrink: 0;
  animation: tg-pulse 1.5s ease-in-out infinite;
}
@keyframes tg-pulse {
  0%, 100% {
    opacity: 0.3;
    transform: scale(0.85);
    box-shadow: 0 0 0 0 var(--tg-primary-soft);
  }
  50% {
    opacity: 1;
    transform: scale(1.1);
    box-shadow: 0 0 0 8px transparent;
  }
}

/* === link cards === */
.link-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
@media (max-width: 700px) {
  .link-grid { grid-template-columns: 1fr; }
}
.link-card {
  display: flex; flex-direction: column; gap: 8px;
  padding: 14px 16px;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-md);
  text-decoration: none;
  color: inherit;
  transition: all var(--tg-trans);
}
.link-card.primary {
  background: linear-gradient(135deg,
    color-mix(in srgb, var(--tg-primary) 8%, var(--tg-bg-elev)),
    var(--tg-bg-elev));
  border-color: color-mix(in srgb, var(--tg-primary) 35%, transparent);
}
a.link-card:hover {
  border-color: var(--tg-primary);
  transform: translateY(-2px);
  box-shadow: var(--tg-shadow-md);
}
.link-card-head {
  display: flex; align-items: center; gap: 8px;
  font-weight: 700; font-size: 13.5px;
  color: var(--tg-primary);
}
.link-card.primary .link-card-head { color: var(--tg-primary); }
.link-card-tag {
  margin-left: auto;
  padding: 2px 8px;
  background: var(--tg-grad-brand);
  color: var(--tg-on-primary);
  font-size: 10.5px; font-weight: 700;
  border-radius: 999px;
  letter-spacing: 0.04em;
}
.link-card-body {
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 11.5px;
  color: var(--tg-text-secondary);
  word-break: break-all;
  background: var(--tg-hover);
  padding: 8px 10px;
  border-radius: 8px;
  line-height: 1.5;
}
.link-card-foot {
  display: flex; align-items: center; gap: 8px;
  font-size: 12px;
  color: var(--tg-text-tertiary);
}
.link-card-foot span { flex: 1; min-width: 0; }
.link-card-foot a.btn-ghost { text-decoration: none; }

/* === help modal === */
.help-modal { width: min(560px, 95vw); max-height: 88vh; display: flex; flex-direction: column; }
.modal-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 18px 22px;
  border-bottom: 1px solid var(--tg-divider);
}
.modal-title {
  font-family: 'Sora', sans-serif;
  font-size: 17px; font-weight: 700; letter-spacing: -0.018em;
}
.modal-body {
  flex: 1; overflow-y: auto;
  padding: 22px;
  display: flex; flex-direction: column; gap: 16px;
}
.modal-foot {
  display: flex; gap: 10px; justify-content: flex-end;
  padding: 14px 22px;
  border-top: 1px solid var(--tg-divider);
}
.help-lead {
  margin: 0;
  padding: 14px 16px;
  background: var(--tg-grad-brand-soft);
  border-radius: var(--tg-radius-md);
  font-size: 13.5px; line-height: 1.6;
  border: 1px solid color-mix(in srgb, var(--tg-primary) 20%, transparent);
}
.help-lead strong {
  background: var(--tg-grad-brand);
  -webkit-background-clip: text; background-clip: text;
  color: transparent;
  font-weight: 700;
}

.help-step {
  display: flex; gap: 14px; align-items: flex-start;
}
.help-step-num {
  width: 32px; height: 32px;
  display: flex; align-items: center; justify-content: center;
  flex-shrink: 0;
  background: var(--tg-grad-brand);
  color: var(--tg-on-primary);
  border-radius: 50%;
  font-family: 'Sora', sans-serif;
  font-weight: 800; font-size: 15px;
  box-shadow: var(--tg-shadow-glow);
}
.help-step h4 {
  font-family: 'Sora', sans-serif;
  font-size: 14.5px; font-weight: 700;
  margin: 4px 0 4px;
  letter-spacing: -0.012em;
}
.help-step p {
  margin: 0 0 4px;
  font-size: 13px; line-height: 1.6;
  color: var(--tg-text-secondary);
}
.help-step p.muted { color: var(--tg-text-tertiary); font-size: 12px; }
.help-step code {
  background: var(--tg-hover);
  padding: 1px 6px; border-radius: 4px;
  font-size: 11.5px;
}

.trouble-h {
  font-family: 'Sora', sans-serif;
  font-size: 14px; font-weight: 700;
  margin: 0 0 4px;
  color: var(--tg-text);
}
.trouble-list {
  margin: 0;
  padding-left: 20px;
  font-size: 13px;
  line-height: 1.7;
  color: var(--tg-text-secondary);
}
.trouble-list li { margin-bottom: 4px; }
.trouble-list strong { color: var(--tg-text); }
.trouble-list code {
  background: var(--tg-hover);
  padding: 1px 5px; border-radius: 4px; font-size: 11.5px;
}

/* === responsive === */
@media (max-width: 600px) {
  .hero { flex-direction: column; align-items: flex-start; padding: 18px 20px; }
  .btn-help { align-self: stretch; justify-content: center; }
  .binding-item { flex-wrap: wrap; }
  .binding-actions { width: 100%; justify-content: flex-end; }
}
</style>
