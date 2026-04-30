<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useDataStore } from '@/stores/data'
import { useNotificationsStore } from '@/stores/notifications'
import { ApiError } from '@/api'
import type { List } from '@/types'
import { installTodoDueScheduler } from '@/scheduler'

const auth = useAuthStore()
const data = useDataStore()
const notif = useNotificationsStore()
const router = useRouter()
const route = useRoute()

const search = ref('')
const sidebarOpen = ref(false)

// ---------- 分类（List）对话框 ----------
const showListDialog = ref(false)
const editingListId = ref<number | null>(null)
const listForm = ref({ name: '', color: '#6366f1' })
const errMsg = ref('')

// 9 种预设色（与 style.css 中 --cat-* 对应）
const PRESET_COLORS = [
  { hex: '#f43f5e', label: '玫红' },
  { hex: '#fb7185', label: '珊瑚' },
  { hex: '#f59e0b', label: '琥珀' },
  { hex: '#10b981', label: '翠绿' },
  { hex: '#14b8a6', label: '蓝绿' },
  { hex: '#0ea5e9', label: '天蓝' },
  { hex: '#6366f1', label: '靛蓝' },
  { hex: '#8b5cf6', label: '紫罗兰' },
  { hex: '#d946ef', label: '桃红' },
]

onMounted(async () => {
  try {
    await Promise.all([data.loadLists(), notif.refreshUnread()])
  } catch {
    /* 401 已被 api 层处理跳走 */
  }
  notif.startSSE()
  installTodoDueScheduler()
})

onBeforeUnmount(() => {
  notif.stopSSE()
})

watch(() => route.fullPath, () => {
  sidebarOpen.value = false
})

const sidebarFilters = [
  { name: 'schedule', label: '日程', icon: 'calendar' },
  { name: 'all', label: '全部任务', icon: 'inbox' },
  { name: 'archive', label: '完成 / 过期', icon: 'archive' },
  { name: 'no-date', label: '无日期', icon: 'circle' },
] as const

const sidebarTools = [
  { name: 'calendar', label: '日历', icon: 'calendar-grid' },
  { name: 'pomodoro', label: '番茄专注', icon: 'tomato' },
  { name: 'stats', label: '数据复盘', icon: 'chart' },
  { name: 'notifications', label: '通知中心', icon: 'bell' },
  { name: 'telegram', label: 'Telegram', icon: 'plane' },
  { name: 'settings', label: '设置', icon: 'gear' },
] as const

const isActive = (name: string) => route.name === name

async function logout() {
  await auth.logout()
  router.replace({ name: 'login' })
}

async function submitSearch() {
  router.push({
    name: route.name as string,
    params: route.params,
    query: { ...route.query, q: search.value || undefined },
  })
}

function openNewList() {
  editingListId.value = null
  listForm.value = { name: '', color: PRESET_COLORS[6].hex }
  errMsg.value = ''
  showListDialog.value = true
}
function openEditList(l: List) {
  editingListId.value = l.id
  listForm.value = { name: l.name, color: l.color || PRESET_COLORS[6].hex }
  errMsg.value = ''
  showListDialog.value = true
}
async function submitList() {
  errMsg.value = ''
  const name = listForm.value.name.trim()
  if (!name) {
    errMsg.value = '名称不能为空'
    return
  }
  try {
    if (editingListId.value === null) {
      await data.createList({ name, color: listForm.value.color })
    } else {
      await data.updateList(editingListId.value, { name, color: listForm.value.color })
    }
    showListDialog.value = false
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}
async function removeList(l: List) {
  if (!confirm(`删除分类 "${l.name}" ？该分类下的任务会变为「未分类」。`)) return
  try {
    await data.removeList(l.id)
    if (route.name === 'list' && Number(route.params.id) === l.id) {
      router.replace({ name: 'schedule' })
    }
  } catch (e) {
    alert((e instanceof ApiError ? e.message : (e as Error).message) || '删除失败')
  }
}

const userInitial = computed(() => {
  const u = auth.user
  if (!u) return '?'
  return (u.display_name || u.email || '?').charAt(0).toUpperCase()
})
const userLabel = computed(() => auth.user?.display_name || auth.user?.email || '')
const userEmail = computed(() => auth.user?.email || '')
</script>

<template>
  <div class="app-shell">
    <div class="sidebar-backdrop" :class="{ 'is-open': sidebarOpen }" @click="sidebarOpen = false"></div>

    <aside class="sidebar" :class="{ 'is-open': sidebarOpen }">
      <div class="sidebar-header">
        <div class="brand">
          <span class="logo-mark" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="20 6 9 17 4 12"></polyline>
            </svg>
          </span>
          <span class="brand-name">TaskFlow</span>
        </div>
      </div>

      <nav>
        <div class="group-title">任务视图</div>
        <RouterLink
          v-for="f in sidebarFilters"
          :key="f.name"
          :to="{ name: f.name }"
          :class="{ active: isActive(f.name) }"
        >
          <span class="nav-icon" v-html="navIcon(f.icon)" />
          <span class="nav-text">{{ f.label }}</span>
        </RouterLink>

        <div class="group-title-row">
          <span>我的分类</span>
          <button class="add-btn" title="新建分类" @click="openNewList">+</button>
        </div>

        <div v-if="data.lists.length === 0" class="cat-empty">
          点 <strong>+</strong> 创建第一个分类
        </div>

        <div v-for="l in data.lists" :key="l.id" class="list-row">
          <RouterLink
            :to="{ name: 'list', params: { id: l.id } }"
            :class="{ active: route.name === 'list' && Number(route.params.id) === l.id }"
          >
            <span class="nav-icon">
              <span class="dot" :style="{ background: l.color || 'var(--tg-primary)', color: l.color || 'var(--tg-primary)' }" />
            </span>
            <span class="nav-text">{{ l.name }}</span>
          </RouterLink>
          <div class="list-actions">
            <button title="重命名 / 改色" @click.stop="openEditList(l)">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
                <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
              </svg>
            </button>
            <button title="删除分类" @click.stop="removeList(l)">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="3 6 5 6 21 6"/>
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
              </svg>
            </button>
          </div>
        </div>

        <div class="group-title">工具</div>
        <RouterLink
          v-for="t in sidebarTools"
          :key="t.name"
          :to="{ name: t.name }"
          :class="{ active: isActive(t.name) }"
        >
          <span class="nav-icon" v-html="navIcon(t.icon)" />
          <span class="nav-text">{{ t.label }}</span>
          <span v-if="t.name === 'notifications' && notif.unread > 0" class="badge danger">{{ notif.unread }}</span>
        </RouterLink>
      </nav>

      <div class="footer">
        <div class="user-card" :title="userEmail">
          <div class="avatar">{{ userInitial }}</div>
          <div class="user-info">
            <div class="user-name">{{ userLabel || '未登录' }}</div>
            <div class="user-meta">{{ userEmail }}</div>
          </div>
        </div>
        <button class="btn-icon" title="退出登录" @click="logout">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
            <polyline points="16 17 21 12 16 7"/>
            <line x1="21" y1="12" x2="9" y2="12"/>
          </svg>
        </button>
      </div>
    </aside>

    <main class="main">
      <header class="topbar">
        <button class="menu-btn" aria-label="打开侧栏" @click="sidebarOpen = !sidebarOpen">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <line x1="3" y1="6" x2="21" y2="6"/>
            <line x1="3" y1="12" x2="21" y2="12"/>
            <line x1="3" y1="18" x2="21" y2="18"/>
          </svg>
        </button>
        <div class="title">{{ pageTitle(route) }}</div>
        <div class="topbar-actions">
          <input v-model="search" type="search" placeholder="搜索任务…" @keydown.enter="submitSearch" />
        </div>
      </header>
      <section class="content">
        <div class="content-inner">
          <router-view v-slot="{ Component }">
            <transition name="fade" mode="out-in">
              <component :is="Component" />
            </transition>
          </router-view>
        </div>
      </section>
    </main>

    <!-- 新建 / 编辑分类 dialog -->
    <Transition name="fade">
      <div v-if="showListDialog" class="modal-backdrop" @click.self="showListDialog = false">
        <div class="modal-card cat-modal">
          <header class="modal-head">
            <span class="modal-title">{{ editingListId === null ? '新建分类' : '编辑分类' }}</span>
            <button class="btn-icon" @click="showListDialog = false" aria-label="关闭">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </header>

          <div class="modal-body">
            <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>

            <!-- 实时预览 -->
            <div class="cat-preview" :style="{ '--cat-color': listForm.color }">
              <span class="dot" />
              <span class="name">{{ listForm.name.trim() || '分类名称' }}</span>
            </div>

            <div class="field">
              <label>名称</label>
              <input v-model="listForm.name" autofocus maxlength="60" placeholder="例如：工作 / 学习 / 生活…" @keydown.enter="submitList" />
            </div>

            <div class="field">
              <label>颜色</label>
              <div class="color-swatches">
                <button
                  v-for="c in PRESET_COLORS"
                  :key="c.hex"
                  type="button"
                  class="color-swatch"
                  :class="{ 'is-selected': listForm.color.toLowerCase() === c.hex.toLowerCase() }"
                  :style="{ background: c.hex }"
                  :title="c.label"
                  @click="listForm.color = c.hex"
                />
                <input
                  v-model="listForm.color"
                  type="color"
                  title="自定义颜色"
                  class="custom-color"
                />
              </div>
            </div>
          </div>

          <footer class="modal-foot">
            <button class="btn-secondary" @click="showListDialog = false">取消</button>
            <button class="btn-primary" @click="submitList">
              {{ editingListId === null ? '创建' : '保存' }}
            </button>
          </footer>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script lang="ts">
import type { RouteLocationNormalizedLoaded } from 'vue-router'

function pageTitle(route: RouteLocationNormalizedLoaded): string {
  const m: Record<string, string> = {
    schedule: '日程',
    archive: '完成 / 过期',
    'no-date': '无日期',
    all: '全部任务',
    list: '分类',
    calendar: '日历',
    pomodoro: '番茄专注',
    stats: '数据复盘',
    notifications: '通知中心',
    telegram: 'Telegram 绑定',
    settings: '设置',
  }
  return m[String(route.name || '')] || ''
}

function navIcon(name: string): string {
  const stroke = `stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"`
  const wrap = (inner: string) =>
    `<svg width="20" height="20" viewBox="0 0 24 24" fill="none" ${stroke}>${inner}</svg>`
  switch (name) {
    case 'calendar':
      return wrap(`<rect x="3" y="4" width="18" height="18" rx="2" ry="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/>`)
    case 'inbox':
      return wrap(`<polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5.45 5.11L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/>`)
    case 'archive':
      return wrap(`<polyline points="21 8 21 21 3 21 3 8"/><rect x="1" y="3" width="22" height="5"/><line x1="10" y1="12" x2="14" y2="12"/>`)
    case 'circle':
      return wrap(`<circle cx="12" cy="12" r="9"/>`)
    case 'calendar-grid':
      return wrap(`<rect x="3" y="4" width="18" height="18" rx="2"/><line x1="3" y1="10" x2="21" y2="10"/><line x1="9" y1="4" x2="9" y2="22"/><line x1="15" y1="4" x2="15" y2="22"/>`)
    case 'tomato':
      return wrap(`<circle cx="12" cy="13" r="8"/><path d="M9 6c1-2 3-3 6-3"/><path d="M12 5v3"/>`)
    case 'chart':
      return wrap(`<line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/>`)
    case 'bell':
      return wrap(`<path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/>`)
    case 'plane':
      return wrap(`<line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/>`)
    case 'gear':
      return wrap(`<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>`)
    default:
      return wrap('')
  }
}
</script>

<style scoped>
.cat-empty {
  margin: 4px 12px 8px;
  padding: 10px 12px;
  font-size: 12px;
  color: var(--tg-text-tertiary);
  background: var(--tg-hover);
  border-radius: var(--tg-radius-sm);
  text-align: center;
  line-height: 1.5;
}
.cat-empty strong {
  display: inline-block;
  width: 18px; height: 18px;
  line-height: 18px;
  text-align: center;
  background: var(--tg-grad-brand);
  color: var(--tg-on-primary);
  border-radius: 5px;
  font-weight: 800;
  margin: 0 2px;
  vertical-align: middle;
}

.modal-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 18px 22px;
  border-bottom: 1px solid var(--tg-divider);
}
.modal-title {
  font-family: 'Sora', sans-serif;
  font-size: 17px; font-weight: 700;
  letter-spacing: -0.018em;
}
.modal-body {
  padding: 22px;
  display: flex; flex-direction: column; gap: 16px;
}
.modal-foot {
  display: flex; gap: 10px; justify-content: flex-end;
  padding: 14px 22px;
  border-top: 1px solid var(--tg-divider);
}
.field { display: flex; flex-direction: column; gap: 8px; }
.field label {
  font-size: 12px; font-weight: 700;
  color: var(--tg-text-secondary);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.cat-modal { width: min(440px, 95vw); }
.cat-preview {
  display: flex; align-items: center; gap: 10px;
  padding: 14px 16px;
  background: color-mix(in srgb, var(--cat-color) 10%, var(--tg-bg-elev));
  border: 1.5px solid color-mix(in srgb, var(--cat-color) 35%, transparent);
  border-radius: var(--tg-radius-md);
  transition: background var(--tg-trans), border-color var(--tg-trans);
}
.cat-preview .dot {
  width: 14px; height: 14px;
  border-radius: 50%;
  background: var(--cat-color);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--cat-color) 18%, transparent);
}
.cat-preview .name {
  font-family: 'Sora', sans-serif;
  font-weight: 700; font-size: 16px;
  color: color-mix(in srgb, var(--cat-color) 70%, var(--tg-text));
  letter-spacing: -0.01em;
}

.custom-color {
  width: 34px; height: 34px;
  padding: 0;
  border-radius: 50%;
  cursor: pointer;
  background: var(--tg-bg-elev);
  border: 2px dashed var(--tg-divider-strong);
}
.custom-color:hover { border-color: var(--tg-primary); }
</style>
