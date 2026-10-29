package onlineworker

import (
	"context"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestChildExitKillsDescendantHoldingStdout(t *testing.T) {
	path, _ := os.Executable()
	pidFile := filepath.Join(t.TempDir(), "pid")
	p, e := Start(context.Background(), path, Bootstrap{WorldName: "descendant", AccountID: pidFile}, Callbacks{})
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("child exit hidden by descendant pipe")
	}
	b, e := os.ReadFile(pidFile)
	if e != nil {
		t.Fatal(e)
	}
	pid, _ := strconv.Atoi(string(b))
	h, e := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if e != nil {
		return
	}
	defer windows.CloseHandle(h)
	result, e := windows.WaitForSingleObject(h, 3000)
	if e != nil || result != windows.WAIT_OBJECT_0 {
		t.Fatal("descendant survived worker job closure", result, e)
	}
}
