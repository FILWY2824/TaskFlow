import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// https://vite.dev/config/
//
// 开发模式下,前端跑在 3001,/api 与 /ws 反代到 Go 后端 8080。
// 生产模式 npm run build 产物 dist/,丢到 nginx 静态目录,/api 与 /ws 由 nginx 反代到后端。
//
// 端口约定:外部 nginx 已经把对应域名指向了本端口(3001),从而 web 端能够直接通过
// 同源相对路径下载 /downloads/ 下的客户端安装包(Android APK / Windows exe)。
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    host: '127.0.0.1',
    port: 3001,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: false,
      },
      '/ws': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: false,
        // SSE 是 HTTP 长连接,不需要 WebSocket 升级
        ws: false,
      },
      '/healthz': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: false,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    target: 'es2020',
  },
})
