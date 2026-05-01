//go:build !windows

package handlers

import (
	"strings"
	"syscall"
)

// collectDisk 在 unix 平台用 statfs 拿数据库所在分区的容量。
func collectDisk(path string) sysDiskInfo {
	out := sysDiskInfo{Path: path}
	if path == "" {
		return out
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		// 文件还没建出来时,试父目录(install 后 data/ 已存在,但 .db 第一次启动前不存在)
		if i := strings.LastIndex(path, "/"); i > 0 {
			parent := path[:i]
			if err2 := syscall.Statfs(parent, &st); err2 != nil {
				return out
			}
		} else {
			return out
		}
	}
	bsize := uint64(st.Bsize)
	out.TotalBytes = uint64(st.Blocks) * bsize
	out.FreeBytes = uint64(st.Bavail) * bsize
	out.UsedBytes = out.TotalBytes - (uint64(st.Bfree) * bsize)
	if out.TotalBytes > 0 {
		out.UsedPercent = float64(out.UsedBytes) / float64(out.TotalBytes) * 100.0
	}
	return out
}
