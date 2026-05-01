//go:build windows

package handlers

// collectDisk 在 Windows 平台暂不读取磁盘容量(避免引入额外的 windows API 依赖)。
// 服务端的部署目标是 Linux 容器/VPS;开发者本地若用 Windows 跑 go test,
// 该字段会全部为 0,但管理面板仍然可用,只是磁盘那一栏显示"未知"。
func collectDisk(path string) sysDiskInfo {
	return sysDiskInfo{Path: path}
}
