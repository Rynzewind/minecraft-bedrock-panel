package addonupdates

import (
	"net/url"
	"strconv"
	"strings"
)

// Download provenance is an untrusted hint. IdentifyDownload verifies the file bytes.
func downloadFileID(raw string) int64 {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return 0
	}
	if !approvedCurseForgeCDN(u) {
		return 0
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "files" {
		return 0
	}
	for _, s := range parts[1:3] {
		if s == "" {
			return 0
		}
		for _, r := range s {
			if r < '0' || r > '9' {
				return 0
			}
		}
	}
	a, err := strconv.ParseInt(parts[1], 10, 32)
	if err != nil {
		return 0
	}
	b, err := strconv.ParseInt(parts[2], 10, 32)
	if err != nil || a < 1 || b < 0 || b > 999 {
		return 0
	}
	return a*1000 + b
}
