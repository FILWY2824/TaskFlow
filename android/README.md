# TaskFlow Android 客户端 (规格 v2.2 阶段 7 + 8)

> Kotlin 2.0 + Compose Material3 + Room + Retrofit + WorkManager。
> minSdk = 26 (Android 8),targetSdk = 35。

## 端定位(规格 §2.2)

- **完整管理端**:登录 / 注册 / 任务清单(7 种过滤) / 编辑 / 子任务 / 提醒 / 通知 / 番茄 / 统计 / Telegram 绑定 / 权限自检 / 设置。
- **本地强提醒(§7)**:AlarmManager 精确闹钟 + 锁屏全屏 Activity + 前台 Service 响铃 / 振动 + 重启自动恢复。
- **离线策略(§4)**:本地 Room 缓存任务 / 列表 / 子任务 / 提醒;断网时进入只读模式,但已同步的提醒仍然在本地按时触发。

## 目录结构

```
android/
├── README.md
├── build.gradle.kts                   # 顶层
├── settings.gradle.kts
├── gradle.properties
├── gradle/libs.versions.toml          # 版本目录
├── app/
│   ├── build.gradle.kts               # AGP 8.7 + Kotlin 2.0 + Compose plugin
│   ├── proguard-rules.pro
│   └── src/main/
│       ├── AndroidManifest.xml        # 全部权限 + 4 个 component 注册
│       ├── res/                       # strings / themes / network_security / xml / mipmap
│       └── java/com/example/todoalarm/
│           ├── TaskFlowApp.kt        # Application + 手写 DI 容器
│           ├── MainActivity.kt        # NavHost
│           ├── ui/
│           │   ├── theme/Theme.kt
│           │   └── screens/           # 12 个 Compose 屏幕 + 8 个 ViewModel
│           │       # Login / Register / Tasks(支持搜索) / TodoEdit / Calendar
│           │       # Notifications / Telegram / Stats / Pomodoro(实时倒计时)
│           │       # Settings / PermissionCheck / AlarmActivity
│           ├── data/
│           │   ├── remote/            # Retrofit + Moshi + AuthInterceptor
│           │   ├── local/             # Room: TodoCache / ListCache / ReminderCache / SyncMeta / AlarmLog
│           │   ├── repository/        # 7 个 Repository(包含 Result<T> 工具)
│           │   └── auth/              # EncryptedSharedPreferences token 持久化
│           ├── alarm/                 # ★ 强提醒核心
│           │   ├── AlarmScheduler.kt           # 注册 setExactAndAllowWhileIdle
│           │   ├── AlarmReceiver.kt            # 系统回调 -> 拉起 Service / Activity
│           │   ├── AlarmForegroundService.kt   # 响铃 / 振动 / wake_lock,90s 安全自停
│           │   ├── AlarmActivity.kt            # 锁屏全屏 Compose 强提醒窗
│           │   └── BootReceiver.kt             # 重启 / 升级后重排
│           ├── sync/SyncWorker.kt     # WorkManager 周期同步 (15min)
│           └── util/                  # 时间格式化 + 权限查询
└── …
```

## 构建

需要 Android Studio Hedgehog (2023.1.1)+ 或 Gradle 8.10+ + Kotlin 2.0 + JDK 17。

```bash
cd android
# 第一次:生成 gradle-wrapper.jar(只要做一次)
./bootstrap.sh                       # 需要本机已装过 gradle;Android Studio 用户可跳过

./gradlew :app:assembleDebug         # 装到模拟器:./gradlew :app:installDebug
./gradlew :app:assembleRelease       # 发布版,会启用 R8 + ProGuard
```

**用 Android Studio 一步到位**:直接 File → Open → 选 `android/` 目录,IDE 会自动下载 wrapper、sync、build。`bootstrap.sh` 可以跳过。

## 后端连接

默认 `BuildConfig.DEFAULT_SERVER_URL = http://10.0.2.2:8080` ——这是 Android 模拟器的"宿主 127.0.0.1"映射。

- **真机调试**:在登录页或设置页把 URL 改成局域网地址(例如 `http://192.168.1.100:8080`)。
  注意:Android 9+ 默认禁止明文 HTTP,本项目在 `network_security_config.xml` 里**只**对 `10.0.2.2 / 127.0.0.1 / localhost` 开了豁免。生产请走 HTTPS。
- **生产**:在登录页填 `https://todo.example.com`,token 持久化在 EncryptedSharedPreferences(AES-256-GCM)。

## 强提醒流程(规格 §7)

```
用户在 Web/Tauri/Android 任意端创建 reminder
    ↓
服务端写库 + 推 SSE
    ↓
SyncWorker(每 15 min)拉到 reminder.created/updated event → GET /api/reminders/{id}
    ↓
upsert 到 reminders_cache + AlarmScheduler.schedule()(setExactAndAllowWhileIdle)
    ↓
到点系统回调 AlarmReceiver
    ↓
1. 启动 AlarmForegroundService(响铃 + 振动 + wake_lock + 持续通知)
2. 启动 AlarmActivity(showWhenLocked + turnScreenOn)
    ↓
用户操作:
   • "停止响铃"  -> 关 Activity + 停 Service
   • "稍后提醒"  -> 仅本地停响铃,5 min 后下次 tick 再触发
   • "完成任务"  -> 在线时调 /api/todos/{id}/complete;
                    离线时仅本地停响铃 + 提示用户联网后重新确认(规格 §4)
```

## 权限矩阵(规格 §6)

| 权限 | 何时需要 | 是否运行时申请 |
|------|----------|----------------|
| `POST_NOTIFICATIONS` | Android 13+ 的所有通知 | ✓ MainActivity 启动时 |
| `SCHEDULE_EXACT_ALARM` / `USE_EXACT_ALARM` | 精确闹钟 | Android 12 用户授权 / 13+ 自动 |
| `USE_FULL_SCREEN_INTENT` | 锁屏全屏弹窗 | Android 14+ 用户授权 |
| `WAKE_LOCK` | Service 持续响铃期间 | 自动 |
| `VIBRATE` / `TURN_SCREEN_ON` / `SHOW_WHEN_LOCKED` | 强提醒 | 自动 |
| `FOREGROUND_SERVICE_SPECIAL_USE` | Service 类型(Android 14+ 强制) | 自动 |
| `RECEIVE_BOOT_COMPLETED` | 重启后恢复 | 自动 |
| 电池优化白名单 | 防止 Doze 延迟提醒 | 由 PermissionCheckScreen 引导 |

`PermissionCheckScreen` 会展示每一项的当前状态,并提供"去授权"按钮跳到对应系统 Settings 页。从 Settings 返回时通过 `Lifecycle.Event.ON_RESUME` 自动刷新状态。

## OEM 兼容性

国产 ROM(MIUI / EMUI / OriginOS / 鸿蒙)对后台 / 锁屏 / 自启动有额外限制,系统级 Settings 里通常需要再放一遍:

- 自启动 / 后台启动
- 锁屏显示
- 后台保活 / 电池白名单
- 通知优先级 / 锁屏可见

`PermissionCheckScreen` 在最下方有提示文字提醒用户去这些系统专有页面做配置。

## 端到端断网验证(对应规格 §4 验收用例)

1. 装 debug apk,登录;
2. 创建一个未来 2 分钟的提醒;
3. 等到点服务端就位 SSE 推过来,Android 端触发 `AlarmScheduler.schedule()`;
4. 关闭 Wi-Fi / 数据;
5. 到点应当:
   - 弹通知;
   - 弹 AlarmActivity(锁屏也要可见);
   - 持续响铃 / 振动直到用户停;
6. 用户点"完成任务",由于离线,UI 显示"离线 — 已停止响铃,联网后请在主界面再次确认完成";
7. 重新连网,在 TasksScreen 主动 refresh;旧 todo 仍然是未完成 → 用户手动勾上 → 同步到服务端。

## 已知限制 / TODO

- RRULE 周期提醒目前服务端已计算好 `next_fire_at`,Android 客户端不做本地 RRULE 解析。后续如果要"完全离线"(从无 next_fire_at 起步),可引入 `com.google.code.findbugs:rfc2445` 或 `org.dmfs.rfc5545.rrule`。
- TodoEdit 里只能加单次提醒(`trigger_at`)。给周期提醒(`rrule`)的 UI 暂未做,但 Web 端可以。Android 端日历也能看到周期任务的 next_fire_at。
- Web Push(浏览器原生通知):DTO 留了 `channel_web_push` 字段但服务端 / Web 端都未实现 Push API + VAPID。优先级低于 Telegram + Android/Windows 强提醒。
