package migration

import (
	"errors"
	"os"
	"strings"
)

func SynchroniseServerIP(path, address string) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	text := string(b)
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
	}
	lines := strings.Split(text, nl)
	found := false
	for i, line := range lines {
		k, _, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == "server-ip" {
			if found {
				return errors.New("duplicate server-ip property")
			}
			lines[i] = "server-ip=" + address
			found = true
		}
	}
	if found {
		text = strings.Join(lines, nl)
	} else {
		if text != "" && !strings.HasSuffix(text, nl) {
			text += nl
		}
		text += "server-ip=" + address + nl
	}
	return os.WriteFile(path, []byte(text), 0600)
}
