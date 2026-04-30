<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useDataStore } from '@/stores/data'
import { useNotificationsStore } from '@/stores/notifications'
import { ApiError } from '@/api'
import type { List } from '@/types'

const auth = useAuthStore()
const data = useDataStore()
const notif = useNotificationsStore()
const router = useRouter()
const route = useRoute()

const search = ref('')
const sidebarOpen = ref(false) // 移动端侧栏开关

// ---- 清单对话框（新建 / 重命名通用）----
const showListDialog = ref(false)
const editingListId = ref<number | null>(null) // null 表示新建
const listForm = ref({ name: '', color: '#3390ec' })
const errMsg = ref('')

onMounted(async () => {
  try {
    await Promise.all([data.loadLists(), notif.refreshUnread()])
  } catch {
    // 401 已被 api 层处理跳走；这里仅吞掉
  }
  notif.startSSE()
  if ('Notification' in window && Notification.permission === 'default') {
    Notification.requestPermission().catch(() => {})
  }
})

onBeforeUnmount(() => {
  notif.stopSSE()
})

// 路由切换时自动关闭移动端侧栏
watch(() => route.fullPath, () => {
  sidebarOpen.value = false
})

const sidebarFilters = [
  { name: 'schedule', label: '日程', icon: 'calendar' },
  { name: 'all', label: '全部', icon: 'inbox' },
  { name: 'archive', label: '完成 & 过期', icon: 'archive' },
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
  // 触发当前 Tasks 视图重新加载（视图监听 route.query.q）
  router.push({ name: route.name as string, params: route.params, query: { ...route.query, q: search.value || undefined } })
}

// ---- 清单对话框 ----
function openNewList() {
  editingListId.value = null
  listForm.value = { name: '', color: '#3390ec' }
  errMsg.value = ''
  showListDialog.value = true
}
function openEditList(l: List) {
  editingListId.value = l.id
  listForm.value = { name: l.name, color: l.color || '#3390ec' }
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
  if (!confirm(`删除清单 "${l.name}" ？该清单下的任务会变为无清单。`)) return
  try {
    await data.removeList(l.id)
    // 如果当前路由就在被删的清单页，跳回 schedule
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
    <!-- Mobile backdrop -->
    <div
      class="sidebar-backdrop"
      :class="{ 'is-open': sidebarOpen }"
      @click="sidebarOpen = false"
    ></div>

    <!-- Sidebar -->
    <aside class="sidebar" :class="{ 'is-open': sidebarOpen }">
      <div class="sidebar-header">
        <div class="brand">
          <span class="logo-mark" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="20 6 9 17 4 12"></polyline>
            </svg>
          </span>
          <span class="brand-name">ToDo List</span>
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
          <span>我的清单</span>
          <button class="add-btn" title="新建清单" @click="openNewList">+</button>
        </div>
        <div
          v-for="l in data.lists"
          :key="l.id"
          class="list-row"
        >
          <RouterLink
            :to="{ name: 'list', params: { id: l.id } }"
            :class="{ active: route.name === 'list' && Number(route.params.id) === l.id }"
          >
            <span class="nav-icon" :style="{ color: l.color || 'var(--tg-primary)' }">
              <span class="dot" />
            </span>
            <span class="nav-text">{{ l.name }}</span>
          </RouterLink>
          <div class="list-actions">
            <button title="重命名" @click.stop="openEditList(l)">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
            </button>
            <button title="删除清单" @click.stop="removeList(l)">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
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
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
        </button>
      </div>
    </aside>

    <main class="main">
      <header class="topbar">
        <button
          class="menu-btn"
          aria-label="打开侧栏"
          @click="sidebarOpen = !sidebarOpen"
        >
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <line x1="3" y1="6" x2="21" y2="6"/>
            <line x1="3" y1="12" x2="21" y2="12"/>
            <line x1="3" y1="18" x2="21" y2="18"/>
          </svg>
        </button>
        <div class="title">
          <span>{{ pageTitle(route) }}</span>
        </div>
        <div class="topbar-actions">
          <input
            v-model="search"
            type="search"
            placeholder="搜索任务…"
            @keydown.enter="submitSearch"
          />
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

    <!-- 新建 / 编辑清单 dialog（用 modal 风格）-->
    <Transition name="fade">
      <div
        v-if="showListDialog"
        class="modal-backdrop"
        @click.self="showListDialog = false"
      >
        <div class="modal-card" style="width: min(380px, 95vw)">
          <header style="display:flex;align-items:center;justify-content:space-between;padding:14px 18px;border-bottom:1px solid var(--tg-divider)">
            <span style="font-size:16px;font-weight:600">{{ editingListId === null ? '新建清单' : '编辑清单' }}</span>
            <button class="btn-icon" @click="showListDialog = false">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </header>
          <div style="padding:18px;display:flex;flex-direction:column;gap:14px">
            <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>
            <div class="field">
              <label style="font-size:12px;font-weight:600;color:var(--tg-primary);margin-bottom:6px;display:block">名称</label>
              <input v-model="listForm.name" autofocus @keydown.enter="submitList" />
            </div>
            <div class="field">
              <label style="font-size:12px;font-weight:600;color:var(--tg-primary);margin-bottom:6px;display:block">颜色</label>
              <input v-model="listForm.color" type="color" />
            </div>
          </div>
          <footer style="display:flex;gap:10px;justify-content:flex-end;padding:12px 18px;border-top:1px solid var(--tg-divider)">
            <button class="btn-secondary" @click="showListDialog = false">取消</button>
            <button class="btn-primary" @click="submitList">{{ editingListId === null ? '创建' : '保存' }}</button>
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
    archive: '完成 & 过期',
    'no-date': '无日期',
    all: '全部任务',
    list: '清单',
    calendar: '日历',
    pomodoro: '番茄专注',
    stats: '数据复盘',
    notifications: '通知中心',
    telegram: 'Telegram 绑定',
    settings: '设置',
  }
  return m[String(route.name || '')] || ''
}

// 用 inline SVG 替换 emoji，跨平台样式更统一。每个图标都是 24x24 viewBox 下的 currentColor 描边。
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
