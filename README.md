# 多用户 TODO + Android/Windows 强提醒 + 全端管理系统：产品与技术开发文档 v2.2

> 用途：本文件可直接发送给 Claude 或其他 AI 编程助手，作为自主开发依据。  
> v2.2 关键修正：Android 与 Windows 端不能只是提醒客户端，也必须具备与 Web 端同等的 TODO 管理后台能力；但暂不支持离线新增/编辑内容，以避免多端不一致。Android/Windows 离线时仍必须能触发已经同步到本地的提醒。

### 多用户 TODO 与强提醒系统：

- Web、Android、Windows 三端都具备完整 TODO 管理能力。
- Android 端使用 Kotlin 原生 App，支持接近系统闹钟级别的本地强提醒；离线时也必须触发已同步的本地提醒。
- Windows 端使用 Tauri/WinUI/WPF/Avalonia 等桌面方案，支持系统通知、托盘、开机启动、本地响铃和本地提醒调度；离线时也必须触发已同步的本地提醒。
- 服务端使用 Go + SQLite WAL，部署在小型 VPS 上，尽量低内存占用。
- 使用 Nginx 反向代理。
- 支持 Telegram Bot 多用户绑定，每个用户只收到自己的 TODO 推送。
- 支持一次性提醒和周期提醒，例如每隔 6 个月提醒一次。
- 功能参考 Todo 清单类应用，支持 Day Todo、快速添加、重复事件、日历视图、子任务、工作量、番茄专注、数据复盘、桌面/锁屏小组件等能力。
