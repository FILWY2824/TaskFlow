package store

import "time"

// DefaultTimezone 是三端和服务端共同采用的默认用户时区。
const DefaultTimezone = "Asia/Shanghai"

func defaultLocation() *time.Location {
	loc, err := time.LoadLocation(DefaultTimezone)
	if err != nil {
		return time.Local
	}
	return loc
}
