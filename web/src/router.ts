import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

// 路由懒加载，首屏只加载 Layout + 默认视图。
const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/Login.vue'),
    meta: { public: true },
  },
  {
    path: '/register',
    name: 'register',
    component: () => import('@/views/Register.vue'),
    meta: { public: true },
  },
  {
    // OAuth 回调中间页:从认证中心 -> 后端 -> 前端的链路终点。
    // 必须 public,因为本地此时还没有 token(就是来换 token 的)。
    // 也必须不要被「已登录就跳走」逻辑拦截 —— 标 standalone。
    path: '/oauth/callback',
    name: 'oauth-callback',
    component: () => import('@/views/OAuthCallback.vue'),
    meta: { public: true, standalone: true },
  },
  {
    // Tauri 强提醒窗口路由，无需认证（由 Rust 侧拉起）
    path: '/alarm',
    name: 'alarm',
    component: () => import('@/views/Alarm.vue'),
    meta: { public: true, standalone: true },
  },
  {
    path: '/',
    component: () => import('@/views/Layout.vue'),
    children: [
      { path: '', redirect: { name: 'schedule' } },
      { path: 'schedule', name: 'schedule', component: () => import('@/views/Tasks.vue'), props: { filterGroup: 'schedule' } },
      { path: 'all', name: 'all', component: () => import('@/views/Tasks.vue'), props: { filter: 'all', titleZh: '全部' } },
      { path: 'no-date', name: 'no-date', component: () => import('@/views/Tasks.vue'), props: { filter: 'no_date', titleZh: '无日期' } },
      { path: 'uncategorized', name: 'uncategorized', component: () => import('@/views/Tasks.vue'), props: { filter: 'no_list', titleZh: '未分类' } },
      { path: 'list/:id', name: 'list', component: () => import('@/views/Tasks.vue'), props: (r) => ({ filter: 'all', listId: Number(r.params.id), titleZh: '清单' }) },
      { path: 'calendar', name: 'calendar', component: () => import('@/views/Calendar.vue') },
      { path: 'day/:date', name: 'day', component: () => import('@/views/DayDetail.vue'), props: true },
      { path: 'pomodoro', name: 'pomodoro', component: () => import('@/views/Pomodoro.vue') },
      { path: 'pomodoro/history', name: 'pomodoro-history', component: () => import('@/views/PomodoroHistory.vue') },
      { path: 'stats', name: 'stats', component: () => import('@/views/Stats.vue') },
      { path: 'notifications', name: 'notifications', component: () => import('@/views/NotificationsView.vue') },
      { path: 'telegram', name: 'telegram', component: () => import('@/views/Telegram.vue') },
      { path: 'settings', name: 'settings', component: () => import('@/views/Settings.vue') },
    ],
  },
  // BUGFIX: 此前 catchAll 重定向到不存在的路由 'today'，任意未匹配的 URL 都会导航失败。
  { path: '/:catchAll(.*)', redirect: { name: 'schedule' } },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const authStore = useAuthStore()
  if (!to.meta.public && !authStore.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  // 公开路由 + 已登录：除了 alarm 这种"独立窗口"，其他公开页跳到默认视图。
  // BUGFIX: 此前这里也是不存在的 'today'，已登录用户访问 /login 会卡住。
  if (to.meta.public && authStore.isAuthenticated && !to.meta.standalone) {
    return { name: 'schedule' }
  }
})

export default router
