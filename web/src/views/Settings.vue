<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { usePrefsStore } from '@/stores/prefs'
import { fmtDateTime } from '@/utils'
import { TIMEZONE_GROUPS, DEFAULT_TIMEZONE } from '@/timezones'
import { ApiError, getApiBase } from '@/api'
import { isTauri, tauri } from '@/tauri'

const auth = useAuthStore()
const prefs = usePrefsStore()
const ok = ref('')
const err = ref('')

// 判断当前运行环境:
//   Web 浏览器 -> 渲染 "Web 通知" 卡片
//   Tauri    -> 渲染 "Windows 通知" 卡片
// (Android 端走原生 Compose Settings,不会经过本文件,所以这里不需要 android 分支)
const isWindows = computed(() => isTauri())

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
    const v = localStorage.getItem('taskflow.theme')
    if (v === 'light' || v === 'dark') return v
  } catch { /* ignore */ }
  return 'auto'
}
function applyTheme(m: ThemeMode) {
  if (m === 'auto') {
    document.documentElement.removeAttribute('data-theme')
    try { localStorage.removeItem('taskflow.theme') } catch { /* ignore */ }
  } else {
    document.documentElement.setAttribute('data-theme', m)
    try { localStorage.setItem('taskflow.theme', m) } catch { /* ignore */ }
  }
}
function chooseTheme(m: ThemeMode) {
  themeMode.value = m
  applyTheme(m)
}

// 下载链接:浏览器直接用 href,Tauri 用系统浏览器打开
function downloadUrl(path: string): string {
  const base = getApiBase()
  if (base) return base + '/downloads/' + path
  return '/downloads/' + path
}

// ---- 检测更新(Tauri / Windows 端) ----
const LOCAL_VERSION = '1.4.1'
type ReleaseManifest = Record<string, { version?: string; filename?: string; notes?: string }>
const releasesManifest = ref<ReleaseManifest | null>(null)
const updateChecking = ref(false)
const updateResult = ref<null | { hasNew: boolean; version?: string; url?: string; notes?: string }>(null)
const updateErr = ref('')

function releaseFilenameFor(platform: 'android' | 'windows', fallbackFilename: string): string {
  return releasesManifest.value?.[platform]?.filename || fallbackFilename
}

function downloadHrefFor(platform: 'android' | 'windows', fallbackFilename: string): string {
  const filename = releaseFilenameFor(platform, fallbackFilename)
  return downloadUrl(`${platform}/${filename}`)
}

const androidDownloadFilename = computed(() => releaseFilenameFor('android', 'TaskFlow-release-unsigned.apk'))
const windowsDownloadFilename = computed(() => releaseFilenameFor('windows', `TaskFlow_${LOCAL_VERSION}_x64-setup.exe`))
const androidDownloadHref = computed(() => downloadHrefFor('android', 'TaskFlow-release-unsigned.apk'))
const windowsDownloadHref = computed(() => downloadHrefFor('windows', `TaskFlow_${LOCAL_VERSION}_x64-setup.exe`))

async function loadReleasesManifest(): Promise<ReleaseManifest | null> {
  if (releasesManifest.value) return releasesManifest.value
  const resp = await fetch(downloadUrl('latest.json'))
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
  const data = await resp.json() as ReleaseManifest
  releasesManifest.value = data
  return data
}

async function checkUpdate() {
  updateChecking.value = true
  updateErr.value = ''
  updateResult.value = null
  try {
    const data = await loadReleasesManifest()
    const platform = 'windows'
    const remote = data?.[platform]
    if (!remote) throw new Error('清单中未找到当前平台的版本信息')
    const hasNew = compareVersions(remote.version, LOCAL_VERSION) > 0
    updateResult.value = {
      hasNew,
      version: remote.version,
      url: remote.filename ? downloadUrl(`${platform}/${remote.filename}`) : undefined,
      notes: remote.notes,
    }
  } catch (e) {
    updateErr.value = e instanceof Error ? e.message : '检测失败'
  } finally {
    updateChecking.value = false
  }
}

function compareVersions(a: string, b: string): number {
  const pa = a.split('.').map(Number)
  const pb = b.split('.').map(Number)
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const va = pa[i] || 0
    const vb = pb[i] || 0
    if (va !== vb) return va - vb
  }
  return 0
}

function openUpdateDownload() {
  const url = updateResult.value?.url
  if (!url) return
  if (isTauri()) {
    tauri.openExternal(url).catch(() => { window.open(url, '_blank') })
  } else {
    window.open(url, '_blank')
  }
}

function openClientDownload(url: string) {
  if (!url) return
  if (isTauri()) {
    tauri.openExternal(url).catch(() => { window.open(url, '_blank') })
    return
  }
  window.location.assign(url)
}

onMounted(() => {
  themeMode.value = readSavedTheme()
  loadReleasesManifest().catch(() => { /* keep fallback download links */ })
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

    <!-- =============================================================
         Web 端通知与提醒（仅在浏览器中渲染）
         =============================================================
         规格 §17 阶段 13:每端只展示自己 scope 的通知开关。这里是 'web' scope。
         Windows / Android 的开关在各自客户端的"设置"里;那些设置不会出现在
         本卡片里,但在数据库 user_preferences(scope='windows' / 'android') 里照常持久化。 -->
    <div v-if="!isWindows" class="settings-card">
      <div class="card-head">
        <h3>Web 通知与提醒</h3>
        <p class="card-hint">
          这一组开关只对当前的<strong>浏览器</strong>生效。Windows / Android 客户端有各自独立的通知设置,
          但所有平台的偏好都会同步保存到你的账户。
        </p>
      </div>
      <div class="card-body">
        <div class="toggle-row">
          <div class="toggle-text">
            <div class="toggle-title">浏览器桌面通知</div>
            <div class="toggle-desc">
              到点时弹出操作系统级通知。需要先授予浏览器通知权限。当前权限：
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
            <div class="toggle-desc">在浏览器右下角弹出小卡片提醒(不依赖系统通知权限)。</div>
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
            <div class="toggle-desc">当任务到达截止时间,在浏览器内本地弹窗提醒。</div>
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
            <div class="toggle-desc">关闭后,倒计时结束会停留在 0:00 等你手动点"完成"。开启时按设定时长直接结束并入库。</div>
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
            <div class="toggle-desc">浏览器 web audio 蜂鸣。需要标签页处于活动状态(后台标签页可能被节流)。</div>
          </div>
          <label class="switch">
            <input type="checkbox" :checked="prefs.pomodoroSound"
              @change="(e) => prefs.set('pomodoroSound', (e.target as HTMLInputElement).checked)" />
            <span class="slider" />
          </label>
        </div>
        <p class="card-hint" style="margin-top: 14px;">
          <strong>说明:</strong> 浏览器是沙箱环境,不能像 Windows / Android 那样在锁屏 / 后台被杀时仍然响铃。
          如果需要"不依赖浏览器开着也能到点叫醒"的强提醒,请使用 Windows 或 Android 客户端。
        </p>
      </div>
    </div>

    <!-- =============================================================
         Windows 端通知与提醒（仅在 Tauri WebView 中渲染）
         =============================================================
         scope='windows':桌面端独有的开关(系统 Toast / 总在最前 / rodio 响铃)。 -->
    <div v-else class="settings-card">
      <div class="card-head">
        <h3>Windows 通知与强提醒</h3>
        <p class="card-hint">
          这一组开关只对当前的 <strong>Windows 桌面端</strong> 生效。Web / Android 客户端有各自独立的通知设置,
          但所有平台的偏好都会同步保存到你的账户。
        </p>
      </div>
      <div class="card-body">
        <div class="toggle-row">
          <div class="toggle-text">
            <div class="toggle-title">系统 Toast 通知</div>
            <div class="toggle-desc">
              到点时通过 Windows 操作中心弹出系统通知,即使应用窗口没有获得焦点也会出现。
              如果系统通知被关掉,请到 <em>Windows 设置 → 系统 → 通知</em> 中重新允许 TaskFlow。
            </div>
          </div>
          <label class="switch">
            <input type="checkbox" :checked="prefs.desktopNotification"
              @change="(e) => prefs.set('desktopNotification', (e.target as HTMLInputElement).checked)" />
            <span class="slider" />
          </label>
        </div>
        <div class="toggle-row">
          <div class="toggle-text">
            <div class="toggle-title">"总在最前"强提醒窗</div>
            <div class="toggle-desc">到点弹出顶置窗口,直到你点"停止"才消失。即使你正在玩游戏 / 看视频也会盖在最上面。</div>
          </div>
          <label class="switch">
            <input type="checkbox" :checked="prefs.alwaysOnTopAlarm"
              @change="(e) => prefs.set('alwaysOnTopAlarm', (e.target as HTMLInputElement).checked)" />
            <span class="slider" />
          </label>
        </div>
        <div class="toggle-row">
          <div class="toggle-text">
            <div class="toggle-title">应用内提示</div>
            <div class="toggle-desc">在窗口右下角弹出小卡片提醒(配合系统 Toast 双保险)。</div>
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
            <div class="toggle-desc">任务到达截止时间时本地弹窗(独立于服务端 reminder)。</div>
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
            <div class="toggle-desc">关闭后,倒计时结束会停留在 0:00 等你手动点"完成"。</div>
          </div>
          <label class="switch">
            <input type="checkbox" :checked="prefs.pomodoroAutoComplete"
              @change="(e) => prefs.set('pomodoroAutoComplete', (e.target as HTMLInputElement).checked)" />
            <span class="slider" />
          </label>
        </div>
        <div class="toggle-row">
          <div class="toggle-text">
            <div class="toggle-title">到点响铃</div>
            <div class="toggle-desc">通过 rodio 播放系统响铃,直到提醒被停止或自动超时。</div>
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
        <p class="muted">TaskFlow · Web v1.4.1</p>
        <p class="muted">多用户 TODO + Android / Windows 强提醒</p>

        <!-- ===== 客户端下载 / 检测更新 ===== -->
        <div class="about-downloads">
          <!-- Windows (Tauri) 端:检测更新 -->
          <template v-if="isWindows">
            <div class="about-downloads-title">检测更新</div>
            <p class="muted" style="font-size:13px;margin-bottom:10px">当前版本 v1.4.1</p>
            <button class="btn-primary btn-sm" :disabled="updateChecking" @click="checkUpdate">
              {{ updateChecking ? '检测中…' : '检查新版本' }}
            </button>
            <div v-if="updateErr" class="banner banner-err" style="margin-top:10px">{{ updateErr }}</div>
            <div v-if="updateResult" style="margin-top:10px">
              <template v-if="updateResult.hasNew">
                <p style="font-size:14px;font-weight:600;color:var(--tg-primary)">
                  发现新版本 v{{ updateResult.version }}
                </p>
                <p v-if="updateResult.notes" class="muted" style="font-size:12.5px;margin:4px 0 8px">{{ updateResult.notes }}</p>
                <button class="btn-primary btn-sm" @click="openUpdateDownload">下载新版本</button>
              </template>
              <template v-else>
                <p class="muted" style="font-size:13px">✓ 当前已是最新版本</p>
              </template>
            </div>
          </template>
          <!-- Web 端:下载客户端 -->
          <template v-else>
            <div class="about-downloads-title">客户端下载</div>
            <div class="about-downloads-grid">
              <!-- Android APK -->
              <a class="dl-card" :href="androidDownloadHref" :download="androidDownloadFilename" @click.prevent="openClientDownload(androidDownloadHref)">
                <span class="dl-icon dl-icon-android" aria-hidden="true">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="6" y1="2" x2="8" y2="5"/><line x1="18" y1="2" x2="16" y2="5"/><path d="M5 9h14v9a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V9Z"/><circle cx="9" cy="13" r="0.6" fill="currentColor"/><circle cx="15" cy="13" r="0.6" fill="currentColor"/><path d="M3 11v5"/><path d="M21 11v5"/><path d="M9 20v2"/><path d="M15 20v2"/></svg>
                </span>
                <span class="dl-text">
                  <span class="dl-name">TaskFlow Android</span>
                  <span class="dl-sub">APK 安装包</span>
                </span>
                <span class="dl-arrow" aria-hidden="true">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 5v14"/><polyline points="6 13 12 19 18 13"/></svg>
                </span>
              </a>
              <!-- Windows 安装包 -->
              <a class="dl-card" :href="windowsDownloadHref" :download="windowsDownloadFilename" @click.prevent="openClientDownload(windowsDownloadHref)">
                <span class="dl-icon dl-icon-windows" aria-hidden="true">
                  <svg viewBox="0 0 24 24" fill="currentColor"><path d="M3 5.5l7.5-1v8H3v-7zM11.5 4.3L21 3v10h-9.5V4.3zM3 13.5h7.5v8L3 20.5v-7zM11.5 13.5H21v8.5l-9.5-1.3v-7.2z"/></svg>
                </span>
                <span class="dl-text">
                  <span class="dl-name">TaskFlow Windows</span>
                  <span class="dl-sub">NSIS 安装包</span>
                </span>
                <span class="dl-arrow" aria-hidden="true">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 5v14"/><polyline points="6 13 12 19 18 13"/></svg>
                </span>
              </a>
            </div>
            <p class="about-downloads-hint muted">
              下载文件来自服务端 releases/ 目录。Windows 用户请下载 .exe 安装包。
            </p>
          </template>
        </div>
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

/* ===== 关于 box · 客户端下载 ===== */
.about-downloads {
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px solid var(--tg-divider);
}
.about-downloads-title {
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--tg-text-secondary);
  margin-bottom: 10px;
}
.about-downloads-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
.dl-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-md);
  text-decoration: none;
  color: var(--tg-text);
  transition: border-color var(--tg-trans-fast),
              background var(--tg-trans-fast),
              transform var(--tg-trans-fast),
              box-shadow var(--tg-trans-fast);
}
.dl-card:hover {
  border-color: color-mix(in srgb, var(--tg-primary) 45%, transparent);
  background: color-mix(in srgb, var(--tg-primary) 5%, var(--tg-bg-elev));
  transform: translateY(-1px);
  box-shadow: var(--tg-shadow-sm);
}
.dl-icon {
  width: 36px; height: 36px;
  display: inline-flex; align-items: center; justify-content: center;
  border-radius: 10px;
  flex-shrink: 0;
  color: #fff;
}
.dl-icon svg { width: 20px; height: 20px; }
.dl-icon-android {
  background: linear-gradient(135deg, #34d399, #10b981);
}
.dl-icon-windows {
  background: linear-gradient(135deg, #38bdf8, #0284c7);
}
.dl-text {
  flex: 1; min-width: 0;
  display: flex; flex-direction: column; gap: 2px;
  overflow: hidden;
}
.dl-name {
  font-family: 'Sora', sans-serif;
  font-size: 13.5px;
  font-weight: 700;
  color: var(--tg-text);
  letter-spacing: -0.005em;
}
.dl-sub {
  font-size: 11px;
  color: var(--tg-text-tertiary);
  font-family: 'JetBrains Mono', monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dl-arrow {
  color: var(--tg-text-tertiary);
  flex-shrink: 0;
  transition: color var(--tg-trans-fast), transform var(--tg-trans-fast);
}
.dl-card:hover .dl-arrow {
  color: var(--tg-primary);
  transform: translateY(2px);
}
.about-downloads-hint {
  margin: 10px 0 0;
  font-size: 11.5px;
  line-height: 1.55;
}
@media (max-width: 600px) {
  .about-downloads-grid { grid-template-columns: 1fr; }
}
</style>
