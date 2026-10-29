//go:build !windows

package onlineworker

import (
	"errors"
	"os"
	"os/exec"
)

func prepare(*exec.Cmd) {}

func contain(*os.Process) (func(), error) {
	return nil, errors.New("Online worker containment requires Windows")
}
