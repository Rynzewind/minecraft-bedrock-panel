//go:build !windows

package online

import "os"

func privateDirectory(path string) error { return os.MkdirAll(path, 0700) }
