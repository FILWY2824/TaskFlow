<script setup lang="ts">
// /oauth/callback 是认证中心 -> 后端 -> 前端 重定向链路的最后一站。
// 后端处理完 token 交换 + userinfo + upsert 用户后,会把用户重定向到这个路由,
// 并把一次性 handoff_code 放在 URL fragment(#code=xxx)里。
// 把 code 放在 fragment 而不是 query —— fragment 不会被发到后端日志/反向代理里,
// 即便我们这里的服务端日志也无法看到 code,handoff 不外泄。
//
// 在这里我们:
//   1) 解析 fragment;失败 -> 跳回 /login?oauth_error=... 让登录页展示。
//   2) POST /api/auth/oauth/finalize {code} 换 access/refresh JWT。
//   3) 取走 sessionStorage 里之前暂存的 redirect 目标(默认 /),replace 过去。
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api'

const router = useRouter()
const auth = useAuthStore()

const message = ref('正在完成登录…')
const errMsg = ref('')

// parseFragment 解析形如 "#code=xxx&error=yyy" 的 hash。空 hash 返回空对象。
function parseFragment(hash: string): Record<string, string> {
  const out: Record<string, string> = {}
  if (!hash || hash.length < 2) return out
  const raw = hash.startsWith('#') ? hash.slice(1) : hash
  for (const part of raw.split('&')) {
    if (!part) continue
    const eq = part.indexOf('=')
    if (eq < 0) {
      out[decodeURIComponent(part)] = ''
    } else {
      out[decodeURIComponent(part.slice(0, eq))] = decodeURIComponent(part.slice(eq + 1))
    }
  }
  return out
}

function popRedirect(): string {
  try {
    const r = sessionStorage.getItem('taskflow.oauth_redirect') || '/'
    sessionStorage.removeItem('taskflow.oauth_redirect')
    return r
  } catch {
    return '/'
  }
}

onMounted(async () => {
  const params = parseFragment(window.location.hash)
  // 立刻清掉 hash,避免用户刷新这一页或点回退时再次触发(handoff 也是一次性的,
  // 第二次 finalize 一定会失败,但保留 hash 看着也累赘)。
  if (window.location.hash) {
    history.replaceState(null, '', window.location.pathname + window.location.search)
  }

  if (params.error) {
    const desc = params.error_description ? `${params.error}: ${params.error_description}` : params.error
    errMsg.value = desc
    // 1.5s 后自动跳回登录页,把错误带过去展示。
    setTimeout(() => {
      void router.replace({ name: 'login', query: { oauth_error: encodeURIComponent(desc) } })
    }, 1500)
    return
  }

  const code = params.code
  if (!code) {
    errMsg.value = '回调链接缺少 code 参数,无法完成登录'
    setTimeout(() => {
      void router.replace({ name: 'login', query: { oauth_error: encodeURIComponent(errMsg.value) } })
    }, 1500)
    return
  }

  try {
    await auth.loginViaOAuth(code)
    message.value = '登录成功,正在跳转…'
    void router.replace(popRedirect())
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
    setTimeout(() => {
      void router.replace({ name: 'login', query: { oauth_error: encodeURIComponent(errMsg.value) } })
    }, 1500)
  }
})
</script>

<template>
  <div class="auth-page">
    <div class="auth-card" aria-busy="true">
      <div class="auth-logo" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="20 6 9 17 4 12"></polyline>
        </svg>
      </div>
      <h2>{{ errMsg ? '登录失败' : '正在登录' }}</h2>
      <div v-if="!errMsg" class="auth-subtitle" style="text-align:center">{{ message }}</div>
      <div v-else class="auth-error">{{ errMsg }}</div>
    </div>
  </div>
</template>
