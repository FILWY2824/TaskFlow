import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// Tauri Windows 客户端复用 ../web 的源码。
//
// 关键点:
//   - root 设为 ../web,让 Vite 直接吃 web/index.html + web/src/。
//   - 输出到 windows/dist,被 src-tauri/tauri.conf.json 引用。
//   - 同时把 @ alias 指到 ../web/src,让 import 'foo from @/...' 正常工作。
//   - base: './' —— Tauri 加载 tauri://localhost/index.html 时,资源用相对路径
//     最稳妥;用绝对路径在某些 webview 版本上会出现 404 / 白屏。
//   - 通过 envPrefix 让 VITE_* / TAURI_ENV_* 环境变量进 import.meta.env。
//
// 注:Tauri 在 dev 模式下会启动这个 vite,在 build 模式下消费 windows/dist。

const webRoot = fileURLToPath(new URL('../web', import.meta.url))

export default defineConfig(async () => ({
  // Tauri 加载产物时使用 tauri://localhost/index.html;相对路径 './' 让
  // <script src="./assets/...">、CSS、字体等都按 index.html 当前位置解析,
  // 避免在打包后白屏。这是 Windows 客户端启动闪退/白屏最常见的元凶之一。
  base: './',
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
  // VITE_TASKFLOW_DEFAULT_SERVER 是关键 —— 打包时把"出厂默认服务端 URL"
  // 烧进去,这样用户安装后第一次启动就能直接连上,不需要先到设置页填地址。
  // 用户可以在设置里改,改的值会被 Rust 侧 config.json 持久化。
  envPrefix: ['VITE_', 'TAURI_ENV_*'],
  build: {
    outDir: fileURLToPath(new URL('./dist', import.meta.url)),
    emptyOutDir: true,
    target:
      process.env.TAURI_ENV_PLATFORM === 'windows' ? 'chrome105' : 'es2020',
    sourcemap: !!process.env.TAURI_ENV_DEBUG,
  },
}))
