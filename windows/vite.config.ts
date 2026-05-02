import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// =============================================================
// windows/vite.config.ts —— Tauri 客户端复用 ../web 源码
//
// 读取的环境变量(全部来自仓库根目录 ../.env,与 server / web / android 共用):
//   PUBLIC_BASE_URL                —— 客户端"出厂默认服务端 URL"
//                                     (运行时用户可在设置里覆盖,持久化到 %APPDATA%/TaskFlow/config.json)
//   VITE_TASKFLOW_DEFAULT_SERVER   —— 单独设置时优先于 PUBLIC_BASE_URL(便于客户端连不同后端)
//   TAURI_DEV_PORT                 —— Tauri dev 时 Vite 监听端口(必须与 tauri.conf.json 的 devUrl 一致)
//
// 关键点:
//   - root 设为 ../web,让 Vite 直接吃 web/index.html + web/src/。
//   - 输出到 windows/dist,被 src-tauri/tauri.conf.json 引用。
//   - base: './' —— Tauri 加载 tauri://localhost/index.html 时,资源用相对路径
//     最稳妥;用绝对路径在某些 webview 版本上会出现 404 / 白屏。
// =============================================================

const projectRoot = fileURLToPath(new URL('..', import.meta.url))
const webRoot = fileURLToPath(new URL('../web', import.meta.url))

export default defineConfig(({ mode }) => {
  // 从根 .env 读所有需要的字段
  const env = loadEnv(mode, projectRoot, ['VITE_', 'PUBLIC_', 'TASKFLOW_', 'TAURI_'])

  // 默认服务端 URL 的取值优先级:
  //   1) VITE_TASKFLOW_DEFAULT_SERVER   显式覆盖
  //   2) TASKFLOW_DEFAULT_SERVER_URL    兼容旧字段
  //   3) PUBLIC_API_URL                 后端 API 域名(前后端分离)
  //   4) PUBLIC_BASE_URL                前端域名(单域名部署兼容回退)
  const defaultServer = (
    env.VITE_TASKFLOW_DEFAULT_SERVER ||
    env.TASKFLOW_DEFAULT_SERVER_URL ||
    env.PUBLIC_API_URL ||
    env.PUBLIC_BASE_URL ||
    ''
  )
    .trim()
    .replace(/\/+$/, '')

  // 把派生值同时塞回 process.env,让 Vite 与 Cargo 都拿得到:
  //   - import.meta.env.VITE_TASKFLOW_DEFAULT_SERVER  (Vue 代码读)
  //   - cargo build.rs std::env::var("TASKFLOW_DEFAULT_SERVER_URL")  (Rust option_env! 读)
  if (defaultServer) {
    process.env.VITE_TASKFLOW_DEFAULT_SERVER = defaultServer
    process.env.TASKFLOW_DEFAULT_SERVER_URL = defaultServer
  }

  const tauriDevPort = parseInt(env.TAURI_DEV_PORT || '1420', 10)

  return {
    envDir: projectRoot,
    // Tauri 加载产物时使用 tauri://localhost/index.html;相对路径 './' 让
    // <script src="./assets/...">、CSS、字体等都按 index.html 当前位置解析,
    // 避免在打包后白屏。
    base: './',
    root: webRoot,
    plugins: [vue()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('../web/src', import.meta.url)),
      },
    },
    clearScreen: false,
    server: {
      port: tauriDevPort,
      strictPort: true,
      host: '127.0.0.1',
      // dev 时把 /api 与 /ws 反代到 PUBLIC_BASE_URL(本地开发默认 127.0.0.1:8080)
      proxy: {
        '/api':     { target: defaultServer || 'http://127.0.0.1:8080', changeOrigin: false },
        '/ws':      { target: defaultServer || 'http://127.0.0.1:8080', changeOrigin: false, ws: false },
        '/healthz': { target: defaultServer || 'http://127.0.0.1:8080' },
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
  }
})
