import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

// 路由懒加载,首屏只加载 Layout + Today 视图。
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
    // Tauri 强提醒窗口路由,无需认证(由 Rust 侧拉起)
    path: '/alarm',
    name: 'alarm',
    component: () => import('@/views/Alarm.vue'),
    meta: { public: true, standalone: true },
  },
  {
    path: '/',
    component: () => import('@/views/Layout.vue'),
    children: [
      { path: '', redirect: { name: 'today' } },
      { path: 'today', name: 'today', component: () => import('@/views/Tasks.vue'), props: { filter: 'today', titleZh: '今日' } },
      { path: 'tomorrow', name: 'tomorrow', component: () => import('@/views/Tasks.vue'), props: { filter: 'tomorrow', titleZh: '明天' } },
      { path: 'this-week', name: 'this-week', component: () => import('@/views/Tasks.vue'), props: { filter: 'this_week', titleZh: '本周' } },
      { path: 'overdue', name: 'overdue', component: () => import('@/views/Tasks.vue'), props: { filter: 'overdue', titleZh: '过期' } },
      { path: 'no-date', name: 'no-date', component: () => import('@/views/Tasks.vue'), props: { filter: 'no_date', titleZh: '无日期' } },
      { path: 'all', name: 'all', component: () => import('@/views/Tasks.vue'), props: { filter: 'all', titleZh: '全部' } },
      { path: 'completed', name: 'completed', component: () => import('@/views/Tasks.vue'), props: { filter: 'completed', titleZh: '已完成' } },
      { path: 'list/:id', name: 'list', component: () => import('@/views/Tasks.vue'), props: (r) => ({ filter: 'all', listId: Number(r.params.id), titleZh: '清单' }) },
      { path: 'calendar', name: 'calendar', component: () => import('@/views/Calendar.vue') },
      { path: 'pomodoro', name: 'pomodoro', component: () => import('@/views/Pomodoro.vue') },
      { path: 'stats', name: 'stats', component: () => import('@/views/Stats.vue') },
      { path: 'notifications', name: 'notifications', component: () => import('@/views/NotificationsView.vue') },
      { path: 'telegram', name: 'telegram', component: () => import('@/views/Telegram.vue') },
      { path: 'settings', name: 'settings', component: () => import('@/views/Settings.vue') },
    ],
  },
  { path: '/:catchAll(.*)', redirect: { name: 'today' } },
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
  // 公开路由 + 已登录:除了 alarm 这种"独立窗口",其他公开页跳到 today
  if (to.meta.public && authStore.isAuthenticated && !to.meta.standalone) {
    return { name: 'today' }
  }
})

export default router
