# TaskFlow 修复清单 & 上线步骤

本次修复一共解决四类问题：

1. **OAuth 登录失败**(`OAUTH_AUTHORIZE_URL` 配置错 + 少 `/#/`)
2. **公网域名硬编码**(改为 `PUBLIC_BASE_URL` 等 env 配置)
3. **Windows 客户端安装后闪退**(asset base、CSP、panic、WebView2、API base 多重原因)
4. **Android 编译失败**(缺 `gradle-wrapper.jar` + 配置目录里有死代码占位)

下面按部署顺序给步骤。

---

## 一、服务端(部署到 Ubuntu)

### 1.1 准备 `.env`

```bash
cd taskflow/server
cp .env.example .env
vim .env
```

最关键的两行:

```env
PUBLIC_BASE_URL=https://taskflow.teamcy.eu.cc
OAUTH_AUTHORIZE_URL=https://teamcy.eu.cc/#/oauth/authorize
```

注意 `OAUTH_AUTHORIZE_URL` 里 **必须有 `/#/`** —— Profile 项目用 hash 路由,
否则浏览器跳过去会 404。其余 OAuth 配置照原样写,**不要再用 `https://https://` 这种双协议头的拼写。**

`OAUTH_REDIRECT_URL` 与 `OAUTH_FRONTEND_REDIRECT_URL` **可以不填**,
新代码会按 `${PUBLIC_BASE_URL}/api/auth/oauth/callback` 与
`${PUBLIC_BASE_URL}/oauth/callback` 自动推导。

### 1.2 在 Profile 后台核对 OAuth 客户端

登录 `https://teamcy.eu.cc/#/admin/oauth-clients`,确保 `TaskFlow_Client_ID` 这条:

- **Redirect URIs 列表** 里**逐字符**包含: `https://taskflow.teamcy.eu.cc/api/auth/oauth/callback`(末尾不要带斜杠)
- **Scopes** 至少包含: `openid profile email`(否则 userinfo 拿不到 email,创建本地用户失败)
- **Status** = active

### 1.3 重启服务

```bash
# systemd 部署
sudo systemctl restart taskflow

# docker-compose
docker compose up -d --build
```

启动日志里搜 "OAUTH_ENABLED" / "PUBLIC_BASE_URL" 应该看到都已生效。
新版本会在配置错误(双 https://、漏协议头)时直接拒绝启动并打印明确的报错。

---

## 二、Windows 客户端(在 Windows 机器上打包)

### 2.1 打包前置

- Windows 10+
- Node.js 20+
- Rust toolchain (`https://rustup.rs/`)
- Visual Studio 2022 Build Tools(C++ 桌面开发工作负载)
- WebView2 Runtime(系统已带,没有就装 Evergreen)

### 2.2 打包步骤

```powershell
cd taskflow\windows
copy .env.example .env
notepad .env       # 把 VITE_TASKFLOW_DEFAULT_SERVER 改成你的公网域名
powershell -ExecutionPolicy Bypass -File build.ps1
```

产物在 `windows\src-tauri\target\release\bundle\` 下,`.msi` 与 `.exe` 各一份。
把它们上传到服务器(例如放到 `taskflow.teamcy.eu.cc/dl/` 路径下)供用户下载。

### 2.3 关于"安装后闪退"

本次重点修了五件事,任何一项缺失都会导致闪退/白屏:

| 修复 | 文件 |
|---|---|
| `vite.config.ts` 加 `base: './'`(否则 webview 找不到 assets) | `windows/vite.config.ts` |
| CSP 放宽到包含 `tauri:` `asset:` `https:` 等(原 CSP 太严会 block 内置脚本) | `windows/src-tauri/tauri.conf.json` |
| `withGlobalTauri: true`(让 webview 拿到 `__TAURI_INTERNALS__`) | 同上 |
| `webviewInstallMode: downloadBootstrapper`(用户没装 WebView2 时安装器会下载) | 同上 |
| `main.rs` 用 panic hook + Windows MessageBox 替代 `expect()`(以后真出错也有提示而非闪退) | `windows/src-tauri/src/main.rs` |
| `web/src/main.ts` 启动时先解析 API base(否则 Tauri 里 fetch 全部走到 `tauri://localhost/api/...` 而不是你的服务器) | `web/src/main.ts` + `web/src/api.ts` |

如果安装后还是闪退,**新版本会写日志到这两个文件**,把它们发给开发者就能定位:

```
%APPDATA%\TaskFlow\fatal.log    ← panic 堆栈
%APPDATA%\TaskFlow\taskflow.log ← 启动期 info/warn/error
```

也可以用 `Win + R` 输入 `%APPDATA%\TaskFlow` 直接看。

### 2.4 关于"OAuth 在 Windows 客户端里"

**已知限制**:Tauri webview 跑在 `tauri://localhost`,跳到 `https://teamcy.eu.cc`
认证完后,认证中心会把用户带回 `https://taskflow.teamcy.eu.cc/oauth/callback#code=...`
—— 这是浏览器,不是 Tauri 客户端。所以现在的 Login 页在 Tauri 里**会用系统默认浏览器打开**,
用户在浏览器里完成登录,但 token 落到的是浏览器的 localStorage,**不会自动同步回 Tauri**。

完整的"桌面 OAuth 同步登录"需要做以下一项才能闭环:
- 注册 `taskflow://` 自定义 URL scheme,并在 Profile 后台把它加到 redirect URI 白名单
- 或者:让 Tauri 客户端在 `127.0.0.1:某固定端口` 起一个临时 HTTP server 接收回调

这两项都要 Profile 端配合(允许新协议/新端口),不在本次修复范围。
**当前 Windows 客户端的本地邮箱密码登录 + 手动填服务器地址** 是完整可用的;
OAuth 在桌面端是"开浏览器登录后回到客户端再次确认"的体验,可用但不优雅。

---

## 三、Android 客户端(在 Windows 机器上打包)

### 3.1 一次性引导

```powershell
cd taskflow\android
powershell -ExecutionPolicy Bypass -File bootstrap.ps1
```

这个脚本会:
1. 优先尝试 `gradle wrapper`(如果你装了 Gradle)
2. 否则下载 Gradle 8.10.2 zip,从中提取 `gradle-wrapper.jar` 放到 `gradle\wrapper\`

完成后再:

```powershell
.\gradlew :app:assembleRelease
```

或者直接 **用 Android Studio 打开 `android\` 目录**,IDE 自带的 wrapper sync 会自动生成 jar 然后开始编译——这是最省事的路径,推荐。

### 3.2 指定默认服务端地址

打包前把这一行写到 `android\local.properties`(已在 `.gitignore` 里):

```
taskflow.default.server.url=https://taskflow.teamcy.eu.cc
```

或者直接传 env:

```powershell
$env:TASKFLOW_DEFAULT_SERVER_URL = 'https://taskflow.teamcy.eu.cc'
.\gradlew :app:assembleRelease
```

新版本会把这个值烧进 `BuildConfig.DEFAULT_SERVER_URL`,用户首次启动 App 时
作为默认服务端地址,可在 App 内设置页修改。

### 3.3 产物

```
android\app\build\outputs\apk\release\TaskFlow-release.apk
```

---

## 四、本次修改的文件清单

```
.env.example                                  增加 PUBLIC_BASE_URL,修正 OAUTH_AUTHORIZE_URL 注释
server/.env.example                           同上,加详细注释说明 hash 路由的坑
server/internal/config/env.go                 LoadOAuthFromEnv 支持 PUBLIC_BASE_URL 推导 + 双协议头/漏 scheme 报错

web/src/main.ts                               启动时先解析 API base,Tauri 场景必须
web/src/api.ts                                增加 setApiBase/getApiBase/absUrl,所有 fetch 走 absUrl
web/src/tauri.ts                              增加 getDefaultServerUrl + bringToFront 命令桥
web/src/views/Login.vue                       OAuth start URL 在 Tauri 里改用绝对地址 + 系统浏览器

windows/.env.example                          新增 — 打包参数 VITE_TASKFLOW_DEFAULT_SERVER
windows/build.ps1                             新增 — Windows 一键打包脚本
windows/vite.config.ts                        加 base: './'
windows/src-tauri/tauri.conf.json             CSP 放宽 + withGlobalTauri + webviewInstallMode
windows/src-tauri/Cargo.toml                  调整 windows crate features 列表
windows/src-tauri/src/main.rs                 panic hook + 文件日志 + MessageBox 兜底,杜绝静默闪退
windows/src-tauri/src/commands.rs             新增 get_default_server_url + bring_window_to_front
windows/src-tauri/src/config.rs               default_server_url 改读 env(打包时烧入)
windows/src-tauri/src/notify.rs               新增 raise_main_window(提醒触发时主窗口置顶)
windows/src-tauri/src/alarm.rs                fire() 时同时调 raise_main_window
windows/src-tauri/capabilities/default.json   加 unminimize / is-visible 权限

android/bootstrap.ps1                         新增 — Windows 上引导 gradle-wrapper.jar
android/app/build.gradle.kts                  DEFAULT_SERVER_URL 改读 env / local.properties
android/gradle/libs.versions.toml             清理无用的 rfc2445 占位条目
```

完整 unified diff 见同目录 `taskflow-fixes.patch`(可直接 `git apply` 到原仓库)。
