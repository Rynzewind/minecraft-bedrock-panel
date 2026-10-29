package onlineworker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// This fixture is the test binary, never a production worker bypass mode.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--descendant" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "--run" {
		var b Bootstrap
		r := bufio.NewReader(os.Stdin)
		line, e := r.ReadBytes('\n')
		if e != nil || json.Unmarshal(line, &b) != nil {
			os.Exit(2)
		}
		if b.WorldName == "malformed" {
			fmt.Println(`{"type":"event","code":"untrusted-secret"}`)
			os.Exit(0)
		}
		if b.WorldName == "descendant" {
			child := exec.Command(os.Args[0], "--descendant")
			child.Stdout = os.Stdout
			child.Stderr = os.Stderr
			if child.Start() != nil {
				os.Exit(3)
			}
			_ = os.WriteFile(b.AccountID, []byte(strconv.Itoa(child.Process.Pid)), 0600)
			fmt.Println(`{"type":"event","code":"advertising"}`)
			os.Exit(0)
		}
		fmt.Println(`{"type":"event","code":"advertising"}`)
		for {
			if _, e = r.ReadByte(); e != nil {
				os.Exit(0)
			}
		}
	}
	os.Exit(m.Run())
}

func TestProcessCloseCancelsBlockedPipeReader(t *testing.T) {
	path, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	ready := make(chan struct{}, 1)
	p, e := Start(context.Background(), path, Bootstrap{WorldName: "fixture"}, Callbacks{Event: func(code string) {
		if code == "advertising" {
			ready <- struct{}{}
		}
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("fixture did not start")
	}
	if e = p.Close(); e != nil {
		t.Fatal(e)
	}
	select {
	case <-p.Done():
	default:
		t.Fatal("child still alive")
	}
}

func TestProcessRejectsUntrustedChildEvent(t *testing.T) {
	path, _ := os.Executable()
	p, e := Start(context.Background(), path, Bootstrap{WorldName: "malformed"}, Callbacks{Event: func(string) { t.Error("untrusted event delivered") }})
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("malformed child not stopped")
	}
	if p.Err() != ErrProtocol {
		t.Fatal("unexpected failure")
	}
}
