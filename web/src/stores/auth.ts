import { defineStore } from 'pinia'
import { auth, clearTokens, loadTokens, loadUser, saveTokens } from '@/api'
import { tauri } from '@/tauri'
import { usePrefsStore } from '@/stores/prefs'
import type { AuthConfig, User } from '@/types'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: loadUser() as User | null,
    initialized: false,
    // 后端 /api/auth/config 的返回。null = 还没拉过。
    // 登录页据此决定显示哪种登录形式;首次进入登录页时拉一次,之后缓存到刷新页面前。
    authConfig: null as AuthConfig | null,
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
    async loadAuthConfig(): Promise<AuthConfig> {
      // 已经拉过就直接返回缓存。
      if (this.authConfig) return this.authConfig
      try {
        this.authConfig = await auth.config()
      } catch {
        // 后端没启用 OAuth 或暂时不可用 —— 退化到本地登录。
        this.authConfig = { oauth_enabled: false }
      }
      return this.authConfig
    },
    async login(_email: string, _password: string): Promise<never> {
      // 三端强制 OAuth 登录;本方法保留为占位,直接抛错以便调用方能立刻发现误用。
      throw new Error('email/password login is disabled — use loginViaOAuth() instead')
    },
    // OAuth 流程的最后一步:浏览器在认证中心走完一圈跳回 /oauth/callback,
    // 前端把 fragment 里的 handoff code 交给后端换本服务的 access/refresh token。
    // Tauri / Android 客户端走"系统浏览器 + poll",拿到 handoff 后也调本方法。
    async loginViaOAuth(handoffCode: string) {
      const r = await auth.oauthFinalize(handoffCode)
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
