package serverupdate

import (
	"regexp"
	"strconv"
	"strings"
)

var versionLine = regexp.MustCompile(`(?m)\bVersion: (1\.[0-9]+\.[0-9]+\.[0-9]+)\s*(?:\r?$)`)

func logVersion(log string) string {
	m := versionLine.FindAllStringSubmatch(log, -1)
	if len(m) == 0 {
		return ""
	}
	return m[len(m)-1][1]
}
func compareVersion(a, b string) int {
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(aa), len(bb)); i++ {
		x, y := 0, 0
		if i < len(aa) {
			x, _ = strconv.Atoi(aa[i])
		}
		if i < len(bb) {
			y, _ = strconv.Atoi(bb[i])
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}
