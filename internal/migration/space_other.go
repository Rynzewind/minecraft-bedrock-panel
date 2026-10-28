//go:build !windows

package migration

import "errors"

func checkSpace(string, uint64) error { return errors.New("migration capture requires Windows") }
