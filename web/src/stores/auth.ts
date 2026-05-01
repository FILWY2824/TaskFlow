import { defineStore } from 'pinia'
import { auth, clearTokens, loadTokens, loadUser, saveTokens } from '@/api'
import { tauri } from '@/tauri'
import { usePrefsStore } from '@/stores/prefs'
import type { User } from '@/types'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: loadUser() as User | null,
    initialized: false,
  }),
  getters: {
    isAuthenticated(state): boolean {
      return !!state.user && !!loadTokens()
    },
    timezone(state): string {
      return state.user?.timezone || 'UTC'
    },
  },
  actions: {
    async login(email: string, password: string) {
      const r = await auth.login({ email, password })
      saveTokens(
        {
          accessToken: r.access_token,
          accessExp: r.access_token_expires_at,
          refreshToken: r.refresh_token,
          refreshExp: r.refresh_token_expires_at,
        },
        r.user,
      )
      this.user = r.user
      // Tauri:把 token 也交给 Rust 后台
      await tauri.setTokens({
        access_token: r.access_token,
        refresh_token: r.refresh_token,
        timezone: r.user.timezone,
      })
      // 登录成功后,从服务端拉一次本端 scope 的偏好(规格 §17)
      void usePrefsStore().hydrate()
    },
    async register(input: {
      email: string
      password: string
      display_name?: string
      timezone?: string
    }) {
      const r = await auth.register(input)
      saveTokens(
        {
          accessToken: r.access_token,
          accessExp: r.access_token_expires_at,
          refreshToken: r.refresh_token,
          refreshExp: r.refresh_token_expires_at,
        },
        r.user,
      )
      this.user = r.user
      await tauri.setTokens({
        access_token: r.access_token,
        refresh_token: r.refresh_token,
        timezone: r.user.timezone,
      })
      void usePrefsStore().hydrate()
    },
    async logout() {
      try {
        const t = loadTokens()
        if (t) await auth.logout(t.refreshToken)
      } catch {
        // 忽略 logout 失败,本地清掉就行
      }
      clearTokens()
      this.user = null
      await tauri.clearTokens()
    },
    async refreshMe() {
      try {
        const me = await auth.me()
        this.user = me
        const t = loadTokens()
        if (t) {
          saveTokens(t, me)
          await tauri.setTokens({
            access_token: t.accessToken,
            refresh_token: t.refreshToken,
            timezone: me.timezone,
          })
        }
        // 刷新页面后再 hydrate 一次 web scope 的偏好
        void usePrefsStore().hydrate()
      } catch {
        // ignore
      }
    },
    async updateProfile(input: { display_name?: string; timezone?: string }) {
      const me = await auth.updateMe(input)
      this.user = me
      const t = loadTokens()
      if (t) {
        saveTokens(t, me)
        await tauri.setTokens({
          access_token: t.accessToken,
          refresh_token: t.refreshToken,
          timezone: me.timezone,
        })
      }
      return me
    },
  },
})
