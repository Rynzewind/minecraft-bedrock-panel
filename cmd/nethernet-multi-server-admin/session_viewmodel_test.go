package main

import (
	"testing"
	"time"

	"nethernet-multi-server/internal/sessionhistory"
)

func TestBuildSessionRowsFlagsDisconnectCluster(t *testing.T) {
	base := time.Date(2026, 9, 22, 17, 30, 0, 0, time.Local)
	events := []sessionhistory.Event{
		{Time: base, Type: sessionhistory.Disconnected, ConsoleGamertag: "One"},
		{Time: base.Add(1500 * time.Millisecond), Type: sessionhistory.Disconnected, ConsoleGamertag: "Two"},
		{Time: base.Add(20 * time.Second), Type: sessionhistory.Disconnected, ConsoleGamertag: "Three"},
	}
	rows := BuildSessionRows(events)
	if rows[0].Alert != "SIMULTANEOUS DROP" || rows[1].Alert != "SIMULTANEOUS DROP" {
		t.Fatalf("cluster not flagged: %+v", rows)
	}
	if rows[2].Alert != "" {
		t.Fatalf("unrelated disconnect flagged: %+v", rows[2])
	}
}
