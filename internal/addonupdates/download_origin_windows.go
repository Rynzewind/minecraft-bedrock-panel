//go:build windows

package addonupdates

import (
	"io"
	"os"
	"strings"
)

func DownloadFileID(path string) int64 {
	f, err := os.Open(path + ":Zone.Identifier")
	if err != nil {
		return 0
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(b) > 16384 {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.EqualFold(key, "HostUrl") {
			return downloadFileID(value)
		}
	}
	return 0
}
