package main

import "testing"

func TestAddonOverviewSortsByResultThenName(t *testing.T) {
	rows := []addonOverviewRow{{UUID: "red", Name: "A", Status: "Outdated"}, {UUID: "amber-z", Name: "Z", Status: "Unknown"}, {UUID: "green", Name: "Z", Status: "Up to date"}, {UUID: "amber-a", Name: "a", Status: "Unknown"}}
	sortAddonOverviewRows(rows)
	for i, id := range []string{"green", "amber-a", "amber-z", "red"} {
		if rows[i].UUID != id {
			t.Fatalf("unexpected order: %+v", rows)
		}
	}
}

func TestCollapsedAddonGroupsKeepOtherResults(t *testing.T) {
	rows := []addonOverviewRow{{UUID: "green", Status: "Up to date"}, {UUID: "amber", Status: "Unknown"}, {UUID: "red", Status: "Outdated"}}
	visible := visibleAddonRows(rows, [4]bool{true, false, false})
	if len(visible) != 2 || visible[0].UUID != "amber" || len(rows) != 3 {
		t.Fatalf("unexpected collapse: %+v", visible)
	}
	if got := visibleAddonRows(rows, [4]bool{true, true, true, true}); len(got) != 0 {
		t.Fatal("all groups did not collapse")
	}
}
