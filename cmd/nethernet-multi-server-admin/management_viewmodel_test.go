package main

import (
	"nethernet-multi-server/internal/contentpacks"
	"nethernet-multi-server/internal/supervisor"
	"reflect"
	"testing"
)

func TestServerActionsFollowState(t *testing.T) {
	stopped := BuildServerRow(supervisor.Status{ID: "creative", Name: "Creative", State: supervisor.Stopped})
	if !stopped.CanStart || stopped.CanStop || stopped.CanRestart {
		t.Fatalf("stopped=%+v", stopped)
	}
	running := BuildServerRow(supervisor.Status{ID: "creative", Name: "Creative", State: supervisor.Running, Players: 2})
	if running.CanStart || !running.CanStop || !running.CanRestart || !running.NeedsDisconnectConfirmation {
		t.Fatalf("running=%+v", running)
	}
}

func TestExternallyRunningUsesOrdinaryRunningLabelWithoutManagedActions(t *testing.T) {
	row := BuildServerRow(supervisor.Status{ID: "creative", Name: "Creative", State: supervisor.ExternallyRunning})
	if row.State != string(supervisor.Running) {
		t.Fatalf("state=%q", row.State)
	}
	if row.CanStart || row.CanStop || row.CanRestart {
		t.Fatalf("unsafe actions enabled: %+v", row)
	}
}
func TestPackRowsPreserveUUIDAndType(t *testing.T) {
	pending := true
	rows := BuildPackRows([]contentpacks.InventoryItem{{StoredPack: contentpacks.StoredPack{Pack: contentpacks.Pack{Name: "Pack", UUID: "u", Type: contentpacks.Behavior, Version: [3]int{1, 2, 3}}}, Servers: map[string]contentpacks.AssignmentState{"creative": {Enabled: false, Pending: &pending}, "survival": {Enabled: true}}}})
	if len(rows) != 1 || rows[0].UUID != "u" || rows[0].Type != "behavior" {
		t.Fatalf("rows=%+v", rows)
	}
	if rows[0].Creative != "Disabled (pending enable)" || rows[0].Survival != "Enabled" {
		t.Fatalf("assignment state=%+v", rows[0])
	}
}

func TestRefreshSelectionUsesStableIDsAfterRowsMove(t *testing.T) {
	accounts := []AccountRow{{ID: "new"}, {ID: "selected"}}
	players := []PlayerRow{{XUID: "selected-player"}}
	servers := []ServerRow{{ID: "survival"}, {ID: "creative"}}
	packs := []PackRow{{UUID: "other"}, {UUID: "selected-pack"}}
	if got := accountRowIndex(accounts, "selected"); got != 1 {
		t.Fatalf("account index=%d", got)
	}
	if got := playerRowIndex(players, "selected-player"); got != 0 {
		t.Fatalf("player index=%d", got)
	}
	if got := serverRowIndex(servers, "creative"); got != 1 {
		t.Fatalf("server index=%d", got)
	}
	if got := packRowIndex(packs, "selected-pack"); got != 1 {
		t.Fatalf("pack index=%d", got)
	}
	if got := packRowIndex(packs, "missing"); got != -1 {
		t.Fatalf("missing index=%d", got)
	}
}

func TestPackSelectionMapsIndexesAndUUIDs(t *testing.T) {
	rows := []PackRow{{UUID: "a"}, {UUID: "b"}, {UUID: "c"}}
	if got := selectedPackUUIDs(rows, []int{2, 0, 99}); !reflect.DeepEqual(got, []string{"c", "a"}) {
		t.Fatalf("UUIDs=%v", got)
	}
	if got := packIndexesByUUID(rows, []string{"b", "c"}); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("indexes=%v", got)
	}
}
