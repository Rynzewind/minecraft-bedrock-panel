package main

import (
	"nethernet-multi-server/internal/supervisor"
	"testing"
)

func TestServerStatusOrderIsStableAcrossMapEnumerations(t *testing.T) {
	a := supervisor.Status{ID: "creative", Name: "Creative"}
	b := supervisor.Status{ID: "survival", Name: "Survival"}
	for _, input := range [][]supervisor.Status{{a, b}, {b, a}} {
		got := BuildServerRows(input)
		if got[0].ID != "creative" || got[1].ID != "survival" {
			t.Fatal(got)
		}
	}
}

func TestServerRefreshDoesNotResetStableRows(t *testing.T) {
	var m serverTableModel
	resets, changes := 0, 0
	m.RowsReset().Attach(func() { resets++ })
	m.RowChanged().Attach(func(int) { changes++ })
	rows := []ServerRow{{ID: "creative", Name: "Creative", State: "running"}, {ID: "survival", Name: "Survival", State: "running"}}
	m.set(rows)
	for i := 0; i < 20; i++ {
		m.set(rows)
	}
	if resets != 1 || changes != 0 {
		t.Fatalf("idle polling reset list: resets=%d changes=%d", resets, changes)
	}
	rows[1].Players = 2
	m.set(rows)
	if resets != 1 || changes != 1 || m.rows[1].Players != 2 {
		t.Fatalf("status change rebuilt list: resets=%d changes=%d rows=%v", resets, changes, m.rows)
	}
	rows[0], rows[1] = rows[1], rows[0]
	m.set(rows)
	if resets != 2 {
		t.Fatal("identity/order change must reset native items")
	}
}
