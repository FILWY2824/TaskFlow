import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// Tauri Windows 客户端复用 ../web 的源码。
//
// 关键点:
//   - root 设为 ../web,让 Vite 直接吃 web/index.html + web/src/。
//   - 输出到 windows/dist,被 src-tauri/tauri.conf.json 引用。
//   - 同时把 @ alias 指到 ../web/src,让 import 'foo from @/...' 正常工作。
//   - 通过 publicDir 引入 windows 自己的 public/ 资源(图标等)。
//
// 注:Tauri 在 dev 模式下会启动这个 vite,在 build 模式下消费 windows/dist。

const webRoot = fileURLToPath(new URL('../web', import.meta.url))

export default defineConfig(async () => ({
  root: webRoot,
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('../web/src', import.meta.url)),
    },
  },
  // Tauri 默认监听 1420(也可改),不要走 strictPort=false 否则 tauri 跟 vite 失联
  clearScreen: false,
  server: {
    port: 1420,
    strictPort: true,
    host: '127.0.0.1',
    // 不需要 proxy,因为 Tauri 客户端配置里写了 server URL 后,前端直接发到那个地址。
    // 但 dev 时为了开发体验,把 /api 也代到本地 8080(后端默认)。
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080', changeOrigin: false },
      '/ws':  { target: 'http://127.0.0.1:8080', changeOrigin: false, ws: false },
      '/healthz': { target: 'http://127.0.0.1:8080' },
    },
  },
  envPrefix: ['VITE_', 'TAURI_ENV_*'],
  build: {
    outDir: fileURLToPath(new URL('./dist', import.meta.url)),
    emptyOutDir: true,
    target:
      process.env.TAURI_ENV_PLATFORM === 'windows' ? 'chrome105' : 'es2020',
    sourcemap: !!process.env.TAURI_ENV_DEBUG,
  },
}))
