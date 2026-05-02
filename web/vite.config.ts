import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// =============================================================
// web/vite.config.ts —— 完全由根目录 .env 驱动
//
// 读取的环境变量(都来自仓库根目录 ../.env):
//   PUBLIC_BASE_URL   —— 后端地址(开发 http://127.0.0.1:8080,生产 https://taskflow.teamcy.eu.cc)
//                        dev 模式下作为 vite proxy 的 target;build 后由同源 nginx 反代,无影响
//   WEB_DEV_HOST      —— vite dev 监听 host(默认 127.0.0.1)
//   WEB_DEV_PORT      —— vite dev 监听端口(默认 3003)
//
// 设计决策:不在子目录维护第二份 .env;Vite 通过 envDir: '..' 读上层 .env。
// =============================================================

const projectRoot = fileURLToPath(new URL('..', import.meta.url))

export default defineConfig(({ mode }) => {
  // VITE_ 前缀给前端代码用;PUBLIC_/WEB_/TASKFLOW_ 前缀仅给本配置文件读
  const env = loadEnv(mode, projectRoot, ['VITE_', 'PUBLIC_', 'WEB_', 'TASKFLOW_'])

  // dev 代理目标:优先 PUBLIC_API_URL(后端域名),回退 PUBLIC_BASE_URL
  const apiTarget = (env.PUBLIC_API_URL || env.PUBLIC_BASE_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '')
  const devHost = env.WEB_DEV_HOST || '127.0.0.1'
  const devPort = parseInt(env.WEB_DEV_PORT || '3003', 10)

  return {
    envDir: projectRoot,
    plugins: [vue()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      host: devHost,
      port: devPort,
      strictPort: true,
      proxy: {
        '/api': {
          target: apiTarget,
          changeOrigin: false,
        },
        '/ws': {
          target: apiTarget,
          changeOrigin: false,
          // SSE 是 HTTP 长连接,不需要 WebSocket 升级
          ws: false,
        },
        '/healthz': {
          target: apiTarget,
          changeOrigin: false,
        },
      },
    },
    build: {
      outDir: 'dist',
      sourcemap: false,
      target: 'es2020',
    },
  }
})
