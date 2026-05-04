# TaskFlow Windows 桌面客户端

## 系统要求

- Windows 10 1809+ 或 Windows 11
- [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)（Windows 11 已内置）

## 下载

| 文件 | 大小 | 说明 |
|------|------|------|
| `TaskFlow_1.4.1_x64-setup.exe` | 构建产物为准 | 中文安装程序 |

## 安装

1. 双击 `.msi` 文件
2. 按向导完成安装
3. 安装目录下的 `data/` 存放所有用户数据（数据库、配置）

## 卸载

- Windows 设置 → 应用 → 找到 TaskFlow → 卸载
- 或控制面板 → 程序和功能 → TaskFlow → 卸载
- 卸载不会删除 `data/` 目录中的用户数据，如需完全清除请手动删除安装目录

## 特性

- 数据便携化：所有数据存在安装目录 `data/` 下，方便迁移备份
- 系统托盘常驻：关闭窗口不退出，右键托盘图标可完全退出
- 强提醒：系统通知 + 全屏弹窗 + 响铃
- 开机自启（可在设置中开关）
- 离线照常触发本地提醒

## 版本

v1.4.1 (built 2026-05-04)
