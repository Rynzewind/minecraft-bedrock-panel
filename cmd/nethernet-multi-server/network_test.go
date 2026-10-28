package main

import (
	"bytes"
	"context"
	"errors"
	"nethernet-multi-server/internal/config"
	"strings"
	"testing"
	"time"
)

func TestNetworkReadinessRecoversAfterInitialDeadline(t *testing.T) {
	var out bytes.Buffer
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	probe := func() error {
		if time.Since(start) < 15*time.Millisecond {
			return errors.New("backend not listening")
		}
		return nil
	}
	if !waitForNetworkReadiness(ctx, probe, &out, 2*time.Millisecond, time.Millisecond) {
		t.Fatal("late backend never enabled advertising", out.String())
	}
	if !strings.Contains(out.String(), "continuing to wait") {
		t.Fatal("initial deadline not exercised", out.String())
	}
}
func TestNetworkReadinessCanCancelAfterDeadline(t *testing.T) {
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if waitForNetworkReadiness(ctx, func() error { return errors.New("wrong IP") }, &out, time.Millisecond, time.Millisecond) {
		t.Fatal("advertised invalid network")
	}
}

func TestManagedNetworkMismatchRequestsRepair(t *testing.T) {
	cfg := config.Config{InterfaceIP: "203.0.113.250", Servers: []config.Server{{Backend: "203.0.113.251:19134", Process: &config.Process{}}}}
	if configuredNetworkAvailable(cfg) == nil {
		t.Fatal("obsolete binding accepted")
	}
}
func TestNetworkDriftClosesAdvertising(t *testing.T) {
	closed := false
	if !stopAdvertisingOnDrift(config.Config{InterfaceIP: "203.0.113.250"}, func() { closed = true }) || !closed {
		t.Fatal("obsolete bindings continued advertising")
	}
}
