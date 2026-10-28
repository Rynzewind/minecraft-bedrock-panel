package main

import (
	"testing"

	"nethernet-multi-server/internal/serversettings"
)

func TestApplyServerSettingNumbersAcceptsSafeWholeNumbers(t *testing.T) {
	got, err := applyServerSettingNumbers(serversettings.Settings{}, "25", "32", "8", "120")
	if err != nil {
		t.Fatalf("applyServerSettingNumbers returned error: %v", err)
	}
	if got.MaxPlayers != 25 || got.ViewDistance != 32 || got.TickDistance != 8 || got.PlayerIdleTimeout != 120 {
		t.Fatalf("unexpected settings: %+v", got)
	}
}

func TestApplyServerSettingNumbersRejectsOutOfRangeValue(t *testing.T) {
	_, err := applyServerSettingNumbers(serversettings.Settings{}, "10", "24", "13", "0")
	if err == nil {
		t.Fatal("expected simulation-distance validation error")
	}
}
