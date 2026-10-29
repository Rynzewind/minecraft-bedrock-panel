//go:build !windows

package onlineguard

import (
	"context"
	"errors"
)

func Inspect(context.Context) (Evidence, error) {
	return Evidence{}, errors.New("Windows firewall inspection is unavailable on this platform")
}
