package migration

import (
	"fmt"
	"golang.org/x/sys/windows"
)

func checkSpace(path string, required uint64) error {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return e
	}
	var available, total, free uint64
	if e = windows.GetDiskFreeSpaceEx(p, &available, &total, &free); e != nil {
		return e
	}
	if available < required {
		return fmt.Errorf("insufficient free space: need at least %d MiB", required>>20)
	}
	return nil
}
