<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useDataStore, type TodoStatusFilter } from '@/stores/data'
import { useNotificationsStore } from '@/stores/notifications'
import { ApiError } from '@/api'
import type { List } from '@/types'
import { installTodoDueScheduler } from '@/scheduler'
import { alertDialog, confirmDialog } from '@/dialogs'

const auth = useAuthStore()
const data = useDataStore()
const notif = useNotificationsStore()
const router = useRouter()
const route = useRoute()

const search = ref('')
const sidebarOpen = ref(false)

// ---------- 顶栏：状态筛选下拉（完成 / 未完成 / 过期 / 全部） ----------
const showStatusMenu = ref(false)
const statusMenuRef = ref<HTMLElement | null>(null)

const STATUS_OPTIONS: { value: TodoStatusFilter; label: string; color: string; icon: string }[] = [
  { value: 'all',     label: '全部',   color: 'var(--tg-text-secondary)', icon: 'list' },
  { value: 'open',    label: '未完成', color: 'var(--tg-primary)',        icon: 'circle' },
  { value: 'done',    label: '已完成', color: 'var(--tg-success)',        icon: 'check' },
  { value: 'expired', label: '已过期', color: 'var(--tg-danger)',         icon: 'alert' },
]

const currentStatus = computed(() =>
  STATUS_OPTIONS.find((o) => o.value === data.statusFilter) || STATUS_OPTIONS[0],
)

function pickStatus(s: TodoStatusFilter) {
  data.setStatusFilter(s)
  showStatusMenu.value = false
}

// 状态筛选按钮在「日程相关」与「无日期」页面都显示
const showsStatusFilter = computed(() => {
  const n = String(route.name || '')
  return n === 'schedule' || n === 'list' || n === 'uncategorized' || n === 'all' || n === 'no-date'
})

// 点击外部时关闭菜单
function onDocClick(e: MouseEvent) {
  if (!showStatusMenu.value) return
  const t = e.target as Node
  if (statusMenuRef.value && !statusMenuRef.value.contains(t)) {
    showStatusMenu.value = false
  }
}

// ---------- 分类管理面板（右侧主页面区域弹出） ----------
const showCatPanel = ref(false)
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
  document.addEventListener('mousedown', onDocClick)
})

onBeforeUnmount(() => {
  notif.stopSSE()
  document.removeEventListener('mousedown', onDocClick)
})

watch(() => route.fullPath, () => {
  sidebarOpen.value = false
})

// 侧栏：移除「全部任务」与「我的分类」整段；分类入口移到顶栏按钮 + 弹层。
// 移除「完成 / 过期」入口（已下线 archive 页面）；状态筛选改用顶栏的状态按钮在每个视图里使用。
const sidebarFilters = [
  { name: 'schedule', label: '日程', icon: 'calendar' },
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

// 管理员专属入口:只有当前账号 is_admin = true 时,侧栏才会渲染这一栏。
// 路由 / 视图本身也都再做了 guard,这里只是 UI 层面隐藏入口。
const adminTools = [
  { name: 'admin', label: '管理面板', icon: 'shield' },
] as const

const showAdminTools = computed(() => !!auth.user?.is_admin)

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

// ---------- 分类管理：打开 / 关闭 / CRUD ----------
function openCatPanel() {
  errMsg.value = ''
  showCatPanel.value = true
}
function closeCatPanel() {
  showCatPanel.value = false
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
  const ok = await confirmDialog({
    title: `删除分类 "${l.name}"？`,
    message: '该分类下的所有任务会自动归入「未分类」。分类本身将被永久删除。',
    confirmText: '删除',
    cancelText: '取消',
    danger: true,
  })
  if (!ok) return
  try {
    await data.removeList(l.id)
    if (route.name === 'list' && Number(route.params.id) === l.id) {
      router.replace({ name: 'uncategorized' })
    }
  } catch (e) {
    await alertDialog({
      title: '删除失败',
      message: (e instanceof ApiError ? e.message : (e as Error).message) || '删除失败',
      confirmText: '知道了',
      danger: true,
    })
  }
}

// 在面板里点击某个分类 → 跳转到对应 list 视图
function gotoList(l: List) {
  router.push({ name: 'list', params: { id: l.id } })
  closeCatPanel()
}
function gotoUncategorized() {
  router.push({ name: 'uncategorized' })
  closeCatPanel()
}
// 「全部分类」：回到 schedule 视图（按日期/状态浏览，不限定分类）
function gotoAllCategories() {
  router.push({ name: 'schedule' })
  closeCatPanel()
}

const userInitial = computed(() => {
  const u = auth.user
  if (!u) return '?'
  return (u.display_name || u.email || '?').charAt(0).toUpperCase()
})
const userLabel = computed(() => auth.user?.display_name || auth.user?.email || '')
const userEmail = computed(() => auth.user?.email || '')

// 顶栏 “分类” 按钮高亮：当前在 list/:id 或 uncategorized 视图时高亮
const inCategoryView = computed(
  () => route.name === 'list' || route.name === 'uncategorized',
)
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

        <template v-if="showAdminTools">
          <div class="group-title">管理</div>
          <RouterLink
            v-for="t in adminTools"
            :key="t.name"
            :to="{ name: t.name }"
            :class="{ active: isActive(t.name) }"
          >
            <span class="nav-icon" v-html="navIcon(t.icon)" />
            <span class="nav-text">{{ t.label }}</span>
            <span class="badge admin-badge" aria-label="管理员入口">ADMIN</span>
          </RouterLink>
        </template>
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
        <div class="title">{{ pageTitle(route, data.lists) }}</div>
        <div class="topbar-actions">
          <!-- 状态筛选（完成 / 未完成 / 过期 / 全部） -->
          <div v-if="showsStatusFilter" ref="statusMenuRef" class="status-menu">
            <button
              class="status-btn"
              :class="{ 'is-active': data.statusFilter !== 'all', 'is-open': showStatusMenu }"
              type="button"
              :style="{ '--st-color': currentStatus.color }"
              :title="`状态：${currentStatus.label}`"
              @click="showStatusMenu = !showStatusMenu"
            >
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                <polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"/>
              </svg>
              <span class="status-btn-label">{{ currentStatus.label }}</span>
              <span class="status-btn-dot" />
              <svg class="status-btn-caret" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="6 9 12 15 18 9"/>
              </svg>
            </button>
            <Transition name="popover">
              <div v-if="showStatusMenu" class="status-pop">
                <div class="status-pop-title">按状态筛选</div>
                <button
                  v-for="o in STATUS_OPTIONS"
                  :key="o.value"
                  type="button"
                  class="status-pop-item"
                  :class="{ 'is-selected': data.statusFilter === o.value }"
                  :style="{ '--st-color': o.color }"
                  @click="pickStatus(o.value)"
                >
                  <span class="status-pop-dot" />
                  <span class="status-pop-label">{{ o.label }}</span>
                  <svg v-if="data.statusFilter === o.value" class="status-pop-check" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="20 6 9 17 4 12"/>
                  </svg>
                </button>
              </div>
            </Transition>
          </div>

          <!-- 分类管理按钮（取代左侧栏的分类列表） -->
          <button
            class="cat-btn"
            :class="{ 'is-active': inCategoryView }"
            type="button"
            @click="openCatPanel"
            title="分类管理"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M3 7h7l2 2h9v11a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V8a1 1 0 0 1 1-1z"/>
            </svg>
            <span class="cat-btn-label">分类</span>
            <span v-if="data.lists.length" class="cat-btn-count">{{ data.lists.length }}</span>
          </button>
          <!-- 搜索框：手写样式 + 内嵌图标，避免使用浏览器原生 search 控件 -->
          <div class="topbar-search" :class="{ 'has-value': !!search }">
            <svg class="topbar-search-icon" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="11" cy="11" r="8"/>
              <line x1="21" y1="21" x2="16.65" y2="16.65"/>
            </svg>
            <input
              v-model="search"
              type="text"
              class="topbar-search-input"
              placeholder="搜索任务…"
              @keydown.enter="submitSearch"
            />
            <button
              v-if="search"
              type="button"
              class="topbar-search-clear"
              aria-label="清空"
              @click="search = ''; submitSearch()"
            >
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                <line x1="18" y1="6" x2="6" y2="18"/>
                <line x1="6" y1="6" x2="18" y2="18"/>
              </svg>
            </button>
          </div>
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

    <!-- ============ 分类管理面板（右侧主页面） ============ -->
    <Transition name="fade">
      <div v-if="showCatPanel" class="modal-backdrop" @click.self="closeCatPanel">
        <div class="modal-card cat-panel">
          <header class="modal-head">
            <span class="modal-title">分类管理</span>
            <button class="btn-icon" @click="closeCatPanel" aria-label="关闭">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </header>

          <div class="modal-body">
            <div class="cat-panel-toolbar">
              <div class="cat-panel-hint muted">
                共 {{ data.lists.length + 1 }} 个分类（含「未分类」）。删除分类时，该分类下的任务会自动归入「未分类」。
              </div>
              <button class="btn-primary cat-new-btn" @click="openNewList">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                  <line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>
                </svg>
                新建分类
              </button>
            </div>

            <ul class="cat-list">
              <!-- 「全部分类」：跳回 schedule 视图，不限定分类 -->
              <li class="cat-list-item is-all" @click="gotoAllCategories">
                <span class="cat-list-dot is-all" />
                <div class="cat-list-info">
                  <div class="cat-list-name">
                    全部分类
                    <span class="cat-list-tag">查看所有任务</span>
                  </div>
                  <div class="cat-list-sub muted">不按分类过滤，按日期 / 状态浏览</div>
                </div>
                <span class="cat-list-arrow" aria-hidden="true">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="9 18 15 12 9 6"/>
                  </svg>
                </span>
              </li>

              <!-- 默认且不可删除的「未分类」 -->
              <li class="cat-list-item is-default" @click="gotoUncategorized">
                <span class="cat-list-dot is-uncat" />
                <div class="cat-list-info">
                  <div class="cat-list-name">
                    未分类
                    <span class="cat-list-tag">默认 · 不可删除</span>
                  </div>
                  <div class="cat-list-sub muted">未指定分类的任务会归入这里</div>
                </div>
                <span class="cat-list-arrow" aria-hidden="true">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="9 18 15 12 9 6"/>
                  </svg>
                </span>
              </li>

              <!-- 用户创建的分类 -->
              <li
                v-for="l in data.lists"
                :key="l.id"
                class="cat-list-item"
                :style="{ '--cat-color': l.color || 'var(--tg-primary)' }"
                @click="gotoList(l)"
              >
                <span class="cat-list-dot" />
                <div class="cat-list-info">
                  <div class="cat-list-name">{{ l.name }}</div>
                </div>
                <div class="cat-list-actions">
                  <button class="btn-ghost" title="重命名 / 改色" @click.stop="openEditList(l)">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
                      <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
                    </svg>
                  </button>
                  <button class="btn-ghost btn-danger" title="删除分类" @click.stop="removeList(l)">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                      <polyline points="3 6 5 6 21 6"/>
                      <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                    </svg>
                  </button>
                </div>
              </li>

              <li v-if="data.lists.length === 0" class="cat-list-empty muted">
                还没有自定义分类。点击右上「新建分类」开始添加。
              </li>
            </ul>
          </div>
        </div>
      </div>
    </Transition>

    <!-- ============ 新建 / 编辑分类 dialog ============ -->
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
              <div class="pretty-input-wrap">
                <input
                  v-model="listForm.name"
                  class="pretty-input"
                  autofocus
                  maxlength="60"
                  placeholder="例如：工作 / 学习 / 生活…"
                  @keydown.enter="submitList"
                />
                <span class="pretty-input-glow" aria-hidden="true" />
              </div>
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
import type { List as _List } from '@/types'

function pageTitle(route: RouteLocationNormalizedLoaded, lists: _List[] = []): string {
  // 在 list/:id 视图下直接显示分类名（用顶栏标题作为唯一指示，避免与正文里的卡片重复）
  if (route.name === 'list') {
    const id = Number(route.params.id)
    const l = lists.find((x) => x.id === id)
    return l ? l.name : '分类'
  }
  // 单日详情：显示「YYYY 年 M 月 D 日 · 周X」
  if (route.name === 'day') {
    const dateStr = String(route.params.date || '')
    const m = dateStr.match(/^(\d{4})-(\d{2})-(\d{2})$/)
    if (m) {
      const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]))
      const dows = ['日', '一', '二', '三', '四', '五', '六']
      return `${d.getFullYear()} 年 ${d.getMonth() + 1} 月 ${d.getDate()} 日 · 周${dows[d.getDay()]}`
    }
    return '日程详情'
  }
  const m: Record<string, string> = {
    schedule: '日程',
    'no-date': '无日期',
    uncategorized: '未分类',
    all: '全部任务',
    list: '分类',
    calendar: '日历',
    pomodoro: '番茄专注',
    stats: '数据复盘',
    notifications: '通知中心',
    telegram: 'Telegram 绑定',
    settings: '设置',
    admin: '管理面板',
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
    case 'shield':
      return wrap(`<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>`)
    default:
      return wrap('')
  }
}
</script>

<style scoped>
/* ====== 顶栏「状态筛选」按钮 + 下拉 ====== */
.status-menu { position: relative; }

.status-btn {
  --st-color: var(--tg-text-secondary);
  display: inline-flex; align-items: center; gap: 8px;
  padding: 8px 12px 8px 14px;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-pill);
  color: var(--tg-text-secondary);
  font-size: 13.5px; font-weight: 600;
  cursor: pointer;
  transition: border-color var(--tg-trans-fast), color var(--tg-trans-fast),
              background var(--tg-trans-fast), transform var(--tg-trans-fast),
              box-shadow var(--tg-trans-fast);
}
.status-btn:hover {
  border-color: var(--st-color);
  color: var(--st-color);
  transform: translateY(-1px);
  box-shadow: var(--tg-shadow-sm);
}
.status-btn:active { transform: translateY(0); }
.status-btn.is-active {
  background: color-mix(in srgb, var(--st-color) 12%, var(--tg-bg-elev));
  border-color: color-mix(in srgb, var(--st-color) 50%, transparent);
  color: var(--st-color);
}
.status-btn.is-open {
  border-color: var(--st-color);
  color: var(--st-color);
  box-shadow:
    0 0 0 4px color-mix(in srgb, var(--st-color) 14%, transparent),
    var(--tg-shadow-sm);
}
.status-btn-dot {
  width: 7px; height: 7px;
  border-radius: 50%;
  background: var(--st-color);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--st-color) 22%, transparent);
}
.status-btn-caret {
  color: var(--tg-text-tertiary);
  transition: transform var(--tg-trans-fast), color var(--tg-trans-fast);
}
.status-btn.is-open .status-btn-caret {
  transform: rotate(180deg);
  color: var(--st-color);
}

.status-pop {
  position: absolute; top: calc(100% + 6px); right: 0;
  min-width: 180px;
  padding: 6px;
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-md);
  box-shadow: var(--tg-shadow-lg);
  z-index: 30;
}
.status-pop-title {
  padding: 8px 12px 6px;
  font-size: 11px; font-weight: 800;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--tg-text-tertiary);
}
.status-pop-item {
  --st-color: var(--tg-text-secondary);
  display: flex; align-items: center; gap: 10px;
  width: 100%;
  padding: 9px 12px;
  background: transparent;
  color: var(--tg-text);
  border: none;
  border-radius: var(--tg-radius-sm);
  font-size: 13.5px; font-weight: 600;
  cursor: pointer;
  text-align: left;
  transition: background var(--tg-trans-fast), color var(--tg-trans-fast);
}
.status-pop-item:hover {
  background: color-mix(in srgb, var(--st-color) 8%, transparent);
  color: var(--st-color);
}
.status-pop-item.is-selected {
  background: color-mix(in srgb, var(--st-color) 12%, transparent);
  color: var(--st-color);
}
.status-pop-dot {
  width: 8px; height: 8px;
  border-radius: 50%;
  background: var(--st-color);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--st-color) 22%, transparent);
  flex-shrink: 0;
}
.status-pop-label { flex: 1; }
.status-pop-check { color: var(--st-color); flex-shrink: 0; }

.popover-enter-active, .popover-leave-active {
  transition: opacity var(--tg-trans-fast), transform var(--tg-trans-fast);
  transform-origin: top right;
}
.popover-enter-from, .popover-leave-to {
  opacity: 0;
  transform: translateY(-4px) scale(0.97);
}

/* ====== 顶栏"分类"按钮 ====== */
.cat-btn {
  display: inline-flex; align-items: center; gap: 8px;
  padding: 8px 14px;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-pill);
  color: var(--tg-text-secondary);
  font-size: 13.5px; font-weight: 600;
  transition: border-color var(--tg-trans-fast), color var(--tg-trans-fast),
              background var(--tg-trans-fast), transform var(--tg-trans-fast),
              box-shadow var(--tg-trans-fast);
}
.cat-btn:hover {
  border-color: var(--tg-primary);
  color: var(--tg-primary);
  transform: translateY(-1px);
  box-shadow: var(--tg-shadow-sm);
}
.cat-btn:active { transform: translateY(0); }
.cat-btn.is-active {
  background: var(--tg-grad-brand-soft);
  border-color: color-mix(in srgb, var(--tg-primary) 50%, transparent);
  color: var(--tg-primary);
}
.cat-btn-count {
  display: inline-flex; align-items: center; justify-content: center;
  min-width: 18px; height: 18px; padding: 0 6px;
  background: var(--tg-primary-soft); color: var(--tg-primary);
  font-size: 11px; font-weight: 700;
  border-radius: 999px;
  font-variant-numeric: tabular-nums;
}
.cat-btn.is-active .cat-btn-count { background: var(--tg-primary); color: var(--tg-on-primary); }

@media (max-width: 600px) {
  .cat-btn-label { display: none; }
  .cat-btn { padding: 8px; }
  .status-btn-label { display: none; }
  .status-btn { padding: 8px 10px; }
}

/* ====== 分类管理面板（弹层） ====== */
.cat-panel { width: min(560px, 95vw); }

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
  max-height: 70vh; overflow-y: auto;
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

.cat-panel-toolbar {
  display: flex; align-items: center; gap: 12px;
  flex-wrap: wrap;
}
.cat-panel-hint { flex: 1; min-width: 0; font-size: 12.5px; line-height: 1.55; }
.cat-new-btn {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 9px 16px;
  font-size: 13px;
  white-space: nowrap;
}

.cat-list {
  display: flex; flex-direction: column; gap: 6px;
  margin: 0; padding: 0; list-style: none;
}
.cat-list-empty {
  padding: 18px; text-align: center;
  font-size: 13px;
  background: var(--tg-hover);
  border-radius: var(--tg-radius-md);
}

.cat-list-item {
  --cat-color: var(--tg-primary);
  display: flex; align-items: center; gap: 12px;
  padding: 12px 14px;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-md);
  cursor: pointer;
  transition: border-color var(--tg-trans-fast), background var(--tg-trans-fast),
              transform var(--tg-trans-fast), box-shadow var(--tg-trans-fast);
  position: relative; overflow: hidden;
}
.cat-list-item::before {
  content: '';
  position: absolute; left: 0; top: 0; bottom: 0;
  width: 3px;
  background: var(--cat-color);
  opacity: 0; transition: opacity var(--tg-trans-fast), width var(--tg-trans-fast);
}
.cat-list-item:hover {
  border-color: color-mix(in srgb, var(--cat-color) 55%, transparent);
  background: color-mix(in srgb, var(--cat-color) 6%, var(--tg-bg-elev));
  transform: translateX(2px);
  box-shadow: var(--tg-shadow-sm);
}
.cat-list-item:hover::before { opacity: 1; width: 5px; }
.cat-list-item:active { transform: translateX(0); }

.cat-list-item.is-default {
  --cat-color: var(--tg-text-tertiary);
  background: linear-gradient(135deg,
    color-mix(in srgb, var(--tg-text-tertiary) 8%, var(--tg-bg-elev)),
    var(--tg-bg-elev));
}

/* "全部分类"项：用品牌色渐变与众不同 */
.cat-list-item.is-all {
  --cat-color: var(--tg-primary);
  background: linear-gradient(135deg,
    color-mix(in srgb, var(--tg-primary) 10%, var(--tg-bg-elev)),
    color-mix(in srgb, var(--tg-accent) 6%, var(--tg-bg-elev)));
  border-color: color-mix(in srgb, var(--tg-primary) 28%, var(--tg-divider));
}
.cat-list-dot.is-all {
  background: var(--tg-grad-brand);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--tg-primary) 22%, transparent);
}

.cat-list-dot {
  width: 14px; height: 14px;
  border-radius: 50%;
  background: var(--cat-color);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--cat-color) 18%, transparent);
  flex-shrink: 0;
}
.cat-list-dot.is-uncat {
  background: repeating-linear-gradient(
    45deg,
    var(--tg-text-tertiary),
    var(--tg-text-tertiary) 3px,
    transparent 3px,
    transparent 6px
  );
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--tg-text-tertiary) 12%, transparent);
}

.cat-list-info { flex: 1; min-width: 0; }
.cat-list-name {
  font-family: 'Sora', sans-serif;
  font-weight: 700; font-size: 15px;
  color: var(--tg-text);
  display: flex; align-items: center; gap: 8px;
  flex-wrap: wrap;
}
.cat-list-tag {
  display: inline-flex; align-items: center;
  padding: 2px 8px;
  background: var(--tg-hover);
  color: var(--tg-text-tertiary);
  font-family: 'Manrope', sans-serif;
  font-size: 10.5px; font-weight: 700;
  letter-spacing: 0.04em;
  border-radius: 999px;
}
.cat-list-sub { font-size: 12px; margin-top: 3px; }

.cat-list-actions {
  display: flex; gap: 4px;
  opacity: 0;
  transform: translateX(8px);
  transition: opacity var(--tg-trans-fast), transform var(--tg-trans-fast);
}
.cat-list-item:hover .cat-list-actions {
  opacity: 1; transform: translateX(0);
}
.cat-list-actions .btn-ghost {
  width: 30px; height: 30px;
  padding: 0;
  display: inline-flex; align-items: center; justify-content: center;
  border-radius: var(--tg-radius-sm);
}

.cat-list-arrow {
  display: inline-flex; align-items: center; justify-content: center;
  color: var(--tg-text-tertiary);
  transition: transform var(--tg-trans-fast), color var(--tg-trans-fast);
}
.cat-list-item:hover .cat-list-arrow {
  color: var(--cat-color);
  transform: translateX(3px);
}

/* ====== 新建/编辑分类 dialog ====== */
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

/* ====== 顶栏搜索（手写样式 / 自定义控件） ====== */
.topbar-search {
  --st-color: var(--tg-text-tertiary);
  position: relative;
  display: inline-flex; align-items: center;
  width: 220px;
  padding: 0 10px 0 36px;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-pill);
  transition: width var(--tg-trans), border-color var(--tg-trans-fast),
              color var(--tg-trans-fast), box-shadow var(--tg-trans-fast),
              background var(--tg-trans-fast);
}
.topbar-search:hover {
  border-color: var(--tg-divider-strong);
  background: color-mix(in srgb, var(--tg-primary) 2%, var(--tg-bg-elev));
}
.topbar-search:focus-within {
  width: 280px;
  border-color: var(--tg-primary);
  box-shadow:
    0 0 0 4px color-mix(in srgb, var(--tg-primary) 14%, transparent),
    inset 0 0 0 1px color-mix(in srgb, var(--tg-primary) 18%, transparent);
}
.topbar-search-icon {
  position: absolute; left: 12px; top: 50%;
  transform: translateY(-50%);
  color: var(--tg-text-tertiary);
  transition: color var(--tg-trans-fast), transform var(--tg-trans-fast);
  pointer-events: none;
}
.topbar-search:focus-within .topbar-search-icon {
  color: var(--tg-primary);
  transform: translateY(-50%) scale(1.05);
}
.topbar-search-input {
  flex: 1; min-width: 0;
  padding: 9px 0;
  background: transparent;
  border: none; outline: none;
  color: var(--tg-text);
  font-family: inherit;
  font-size: 13.5px; font-weight: 500;
  caret-color: var(--tg-primary);
}
.topbar-search-input::placeholder {
  color: var(--tg-text-tertiary);
  font-weight: 400;
  transition: opacity var(--tg-trans-fast);
}
.topbar-search-input:focus::placeholder { opacity: 0.55; }
.topbar-search-clear {
  display: inline-flex; align-items: center; justify-content: center;
  width: 22px; height: 22px;
  margin-left: 4px;
  padding: 0;
  background: var(--tg-hover);
  color: var(--tg-text-tertiary);
  border: none;
  border-radius: 50%;
  cursor: pointer;
  transition: background var(--tg-trans-fast), color var(--tg-trans-fast),
              transform var(--tg-trans-fast);
}
.topbar-search-clear:hover {
  background: color-mix(in srgb, var(--tg-danger) 14%, var(--tg-hover));
  color: var(--tg-danger);
  transform: scale(1.08);
}

@media (max-width: 720px) {
  .topbar-search { width: 150px; }
  .topbar-search:focus-within { width: 200px; }
}
@media (max-width: 540px) {
  .topbar-search { display: none; }
}
</style>
