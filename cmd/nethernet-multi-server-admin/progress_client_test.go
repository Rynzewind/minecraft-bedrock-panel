package main

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestProgressCallCancellationInterruptsBlockedPipeRead(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	ctx, cancel := context.WithCancel(context.Background())
	release := bindCallContext(ctx, a)
	defer release()
	done := make(chan error, 1)
	go func() { buf := make([]byte, 1); _, err := a.Read(buf); done <- err }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("read succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled progress read remained blocked")
	}
}
