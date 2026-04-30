<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { usePrefsStore } from '@/stores/prefs'
import { fmtDateTime } from '@/utils'
import { TIMEZONE_GROUPS, DEFAULT_TIMEZONE } from '@/timezones'
import { ApiError } from '@/api'

const auth = useAuthStore()
const prefs = usePrefsStore()
const ok = ref('')
const err = ref('')

// ---- 时区 ----
const tz = ref<string>(auth.user?.timezone || DEFAULT_TIMEZONE)
const tzSaving = ref(false)
const tzDirty = computed(() => tz.value !== (auth.user?.timezone || DEFAULT_TIMEZONE))

async function saveTimezone() {
  err.value = ''
  ok.value = ''
  tzSaving.value = true
  try {
    await auth.updateProfile({ timezone: tz.value })
    ok.value = '时区已更新'
  } catch (e) {
    err.value = e instanceof ApiError ? e.message : (e as Error).message
  } finally {
    tzSaving.value = false
  }
}

// ---- 显示名 ----
const displayName = ref(auth.user?.display_name || '')
const nameSaving = ref(false)
const nameDirty = computed(() => displayName.value !== (auth.user?.display_name || ''))

async function saveDisplayName() {
  err.value = ''
  ok.value = ''
  nameSaving.value = true
  try {
    await auth.updateProfile({ display_name: displayName.value })
    ok.value = '显示名已更新'
  } catch (e) {
    err.value = e instanceof ApiError ? e.message : (e as Error).message
  } finally {
    nameSaving.value = false
  }
}

// ---- 主题 ----
type ThemeMode = 'auto' | 'light' | 'dark'
const themeMode = ref<ThemeMode>('auto')

function readSavedTheme(): ThemeMode {
  try {
    const v = localStorage.getItem('todolist.theme')
    if (v === 'light' || v === 'dark') return v
  } catch { /* ignore */ }
  return 'auto'
}
function applyTheme(m: ThemeMode) {
  if (m === 'auto') {
    document.documentElement.removeAttribute('data-theme')
    try { localStorage.removeItem('todolist.theme') } catch { /* ignore */ }
  } else {
    document.documentElement.setAttribute('data-theme', m)
    try { localStorage.setItem('todolist.theme', m) } catch { /* ignore */ }
  }
}
function chooseTheme(m: ThemeMode) {
  themeMode.value = m
  applyTheme(m)
}

onMounted(() => {
  themeMode.value = readSavedTheme()
})

// ---- 浏览器桌面通知权限 ----
const perm = ref<string>(('Notification' in window) ? Notification.permission : 'unsupported')
const supportsNotification = computed(() => 'Notification' in window)

async function requestPerm() {
  if (!('Notification' in window)) return
  await Notification.requestPermission()
  perm.value = Notification.permission
  if (perm.value !== 'granted') {
    prefs.set('desktopNotification', false)
  }
}

const permLabel = computed(() => {
  switch (perm.value) {
    case 'granted': return '已授权'
    case 'denied': return '已拒绝'
    case 'default': return '未授权'
    default: return '不支持'
  }
})

async function toggleDesktopNotification(v: boolean) {
  if (v && supportsNotification.value && perm.value !== 'granted') {
    await requestPerm()
    if (perm.value !== 'granted') return
  }
  prefs.set('desktopNotification', v)
}
</script>

<template>
  <div class="settings-page">
    <Transition name="fade">
      <div v-if="ok" class="banner banner-ok">{{ ok }}</div>
    </Transition>
    <Transition name="fade">
      <div v-if="err" class="banner banner-err">{{ err }}</div>
    </Transition>

    <div class="settings-card">
      <div class="card-head"><h3>账号信息</h3></div>
      <div class="card-body">
        <div class="form-row">
          <label>邮箱</label>
          <div class="form-static">{{ auth.user?.email }}</div>
        </div>
        <div class="form-row">
          <label>显示名</label>
          <div class="form-input-wrap">
            <div class="pretty-input-wrap" style="flex:1">
              <input v-model="displayName" class="pretty-input" placeholder="未设置" maxlength="64" />
              <span class="pretty-input-glow" aria-hidden="true" />
            </div>
            <button class="btn-primary btn-sm" :disabled="!nameDirty || nameSaving" @click="saveDisplayName">
              {{ nameSaving ? '保存中…' : '保存' }}
            </button>
          </div>
        </div>
        <div class="form-row">
          <label>注册时间</label>
          <div class="form-static muted">{{ fmtDateTime(auth.user?.created_at) }}</div>
        </div>
      </div>
    </div>

    <div class="settings-card">
      <div class="card-head">
        <h3>时区</h3>
        <p class="card-hint">所有任务的截止时间、提醒触发时间都按此时区显示。新建任务时不再单独询问，统一用这里的设置。</p>
      </div>
      <div class="card-body">
        <div class="form-row">
          <label>当前时区</label>
          <div class="form-input-wrap">
            <div class="pretty-input-wrap" style="flex:1">
              <select v-model="tz" class="pretty-input select-lg">
                <optgroup v-for="g in TIMEZONE_GROUPS" :key="g.label" :label="g.label">
                  <option v-for="o in g.options" :key="o.value" :value="o.value">{{ o.label }}</option>
                </optgroup>
              </select>
              <span class="pretty-input-glow" aria-hidden="true" />
            </div>
            <button class="btn-primary btn-sm" :disabled="!tzDirty || tzSaving" @click="saveTimezone">
              {{ tzSaving ? '保存中…' : '保存' }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <div class="settings-card">
      <div class="card-head">
        <h3>外观</h3>
        <p class="card-hint">选择浅色 / 深色，或跟随系统。</p>
      </div>
      <div class="card-body">
        <div class="theme-options">
          <button type="button" class="theme-option preview-auto" :class="{ 'is-selected': themeMode === 'auto' }" @click="chooseTheme('auto')">
            <div class="preview" /><span>跟随系统</span>
          </button>
          <button type="button" class="theme-option preview-light" :class="{ 'is-selected': themeMode === 'light' }" @click="chooseTheme('light')">
            <div class="preview" /><span>浅色</span>
          </button>
          <button type="button" class="theme-option preview-dark" :class="{ 'is-selected': themeMode === 'dark' }" @click="chooseTheme('dark')">
            <div class="preview" /><span>深色</span>
          </button>
        </div>
      </div>
    </div>

    <div class="settings-card">
      <div class="card-head">
        <h3>提醒与通知</h3>
        <p class="card-hint">所有提醒类开关都在这里统一管理，其他页面会按这里的设置走。</p>
      </div>
      <div class="card-body">
        <div class="toggle-row">
          <div class="toggle-text">
            <div class="toggle-title">桌面系统通知</div>
            <div class="toggle-desc">
              到点时弹出操作系统级通知。当前权限：
              <span :class="perm === 'granted' ? 'success-text' : (perm === 'denied' ? 'danger-text' : 'muted')">{{ permLabel }}</span>
              <button v-if="supportsNotification && perm === 'default'" class="btn-ghost btn-xs" style="margin-left:6px" @click="requestPerm">请求权限</button>
            </div>
          </div>
          <label class="switch">
            <input type="checkbox" :checked="prefs.desktopNotification" :disabled="!supportsNotification || perm === 'denied'"
              @change="(e) => toggleDesktopNotification((e.target as HTMLInputElement).checked)" />
            <span class="slider" />
          </label>
        </div>
        <div class="toggle-row">
          <div class="toggle-text">
            <div class="toggle-title">应用内提示</div>
            <div class="toggle-desc">在右下角弹出小卡片提醒。</div>
          </div>
          <label class="switch">
            <input type="checkbox" :checked="prefs.inAppToast"
              @change="(e) => prefs.set('inAppToast', (e.target as HTMLInputElement).checked)" />
            <span class="slider" />
          </label>
        </div>
        <div class="toggle-row">
          <div class="toggle-text">
            <div class="toggle-title">任务截止本地提醒</div>
            <div class="toggle-desc">当任务到达截止时间，本地弹窗提醒。</div>
          </div>
          <label class="switch">
            <input type="checkbox" :checked="prefs.todoDueToast"
              @change="(e) => prefs.set('todoDueToast', (e.target as HTMLInputElement).checked)" />
            <span class="slider" />
          </label>
        </div>
        <div class="toggle-row">
          <div class="toggle-text">
            <div class="toggle-title">番茄到点自动结束</div>
            <div class="toggle-desc">关闭后，倒计时结束会停留在 0:00 等你手动点"完成"。开启时按设定时长直接结束并入库。</div>
          </div>
          <label class="switch">
            <input type="checkbox" :checked="prefs.pomodoroAutoComplete"
              @change="(e) => prefs.set('pomodoroAutoComplete', (e.target as HTMLInputElement).checked)" />
            <span class="slider" />
          </label>
        </div>
        <div class="toggle-row">
          <div class="toggle-text">
            <div class="toggle-title">番茄到点声音</div>
            <div class="toggle-desc">仅在桌面端 (Tauri) 生效。</div>
          </div>
          <label class="switch">
            <input type="checkbox" :checked="prefs.pomodoroSound"
              @change="(e) => prefs.set('pomodoroSound', (e.target as HTMLInputElement).checked)" />
            <span class="slider" />
          </label>
        </div>
      </div>
    </div>

    <div class="settings-card">
      <div class="card-head"><h3>关于</h3></div>
      <div class="card-body">
        <p class="muted">ToDo List · Web v0.4.0</p>
        <p class="muted">多用户 TODO + Android / Windows 强提醒</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.settings-page { max-width: 760px; margin: 0 auto; display: flex; flex-direction: column; gap: 18px; }

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
.card-body { padding: 6px 20px 14px; display: flex; flex-direction: column; }

.form-row {
  display: grid;
  grid-template-columns: 110px 1fr;
  align-items: center;
  gap: 16px;
  padding: 12px 0;
  border-bottom: 1px solid var(--tg-divider);
}
.form-row:last-child { border-bottom: none; }
.form-row label { font-size: 13px; color: var(--tg-text-secondary); font-weight: 500; }
.form-static { font-size: 14px; }
.form-input-wrap { display: flex; gap: 8px; align-items: center; }
/* 直接子元素的兜底样式（pretty-input-wrap 包裹时不参与） */
.form-input-wrap > input,
.form-input-wrap > select.select-lg {
  flex: 1;
  padding: 8px 12px;
  border-radius: var(--tg-radius-sm);
  border: 1.5px solid var(--tg-divider);
  background: var(--tg-bg);
  font-size: 14px;
  color: var(--tg-text);
  transition: border-color var(--tg-trans-fast);
}
.form-input-wrap > input:focus,
.form-input-wrap > select.select-lg:focus { border-color: var(--tg-primary); outline: none; }

.btn-sm { padding: 7px 14px !important; font-size: 13px !important; }
.btn-xs { padding: 3px 8px !important; font-size: 11.5px !important; }

.toggle-row {
  display: flex; align-items: center; justify-content: space-between;
  gap: 16px; padding: 14px 0; border-bottom: 1px solid var(--tg-divider);
}
.toggle-row:last-child { border-bottom: none; }
.toggle-text { flex: 1; min-width: 0; }
.toggle-title { font-size: 14px; font-weight: 600; margin-bottom: 2px; }
.toggle-desc { font-size: 12.5px; color: var(--tg-text-secondary); line-height: 1.5; }

.switch { position: relative; display: inline-block; width: 44px; height: 26px; flex-shrink: 0; }
.switch input { display: none; }
.switch .slider {
  position: absolute; cursor: pointer; inset: 0;
  background: var(--tg-divider-strong); border-radius: 999px;
  transition: background var(--tg-trans-fast);
}
.switch .slider::before {
  position: absolute; content: '';
  height: 20px; width: 20px; left: 3px; top: 3px;
  background: #fff; border-radius: 50%;
  transition: transform var(--tg-trans);
  box-shadow: 0 2px 4px rgba(0,0,0,0.2);
}
.switch input:checked + .slider { background: var(--tg-primary); }
.switch input:checked + .slider::before { transform: translateX(18px); }
.switch input:disabled + .slider { opacity: 0.5; cursor: not-allowed; }

.theme-options { display: flex; gap: 12px; flex-wrap: wrap; padding: 8px 0; }
.theme-option {
  display: flex; flex-direction: column; align-items: center; gap: 8px;
  padding: 12px 18px;
  background: var(--tg-hover);
  border: 1.5px solid transparent;
  border-radius: var(--tg-radius-md);
  cursor: pointer;
  font-size: 13px; font-weight: 500;
  min-width: 100px;
  color: var(--tg-text);
  transition: all var(--tg-trans-fast);
}
.theme-option:hover { background: var(--tg-press); }
.theme-option.is-selected { border-color: var(--tg-primary); background: var(--tg-primary-soft); color: var(--tg-primary); }
.theme-option .preview { width: 44px; height: 28px; border-radius: 6px; border: 1px solid var(--tg-divider); }
.preview-auto .preview { background: linear-gradient(90deg, #fff 50%, #1a1a1a 50%); }
.preview-light .preview { background: #fff; }
.preview-dark .preview { background: #1a1a1a; border-color: #333; }

@media (max-width: 600px) {
  .form-row { grid-template-columns: 1fr; gap: 6px; }
}
</style>
