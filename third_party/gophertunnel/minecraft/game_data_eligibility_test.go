package minecraft

import (
	"reflect"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestGameDataCarriesWorldEligibilityFlags(t *testing.T) {
	typ := reflect.TypeOf(GameData{})
	for _, field := range []string{"AchievementsDisabled", "CommandsEnabled", "EducationFeaturesEnabled"} {
		if _, ok := typ.FieldByName(field); !ok {
			t.Errorf("GameData is missing %s, so a proxy cannot preserve the backend world flag", field)
		}
	}
}

func TestHandleStartGamePreservesWorldEligibilityFlags(t *testing.T) {
	conn := &Conn{}
	pk := &packet.StartGame{
		AchievementsDisabled:     true,
		CommandsEnabled:          false,
		EducationFeaturesEnabled: true,
	}
	if err := conn.handleStartGame(pk); err != nil {
		t.Fatalf("handle StartGame: %v", err)
	}
	if conn.gameData.AchievementsDisabled != pk.AchievementsDisabled {
		t.Errorf("AchievementsDisabled = %v, want %v", conn.gameData.AchievementsDisabled, pk.AchievementsDisabled)
	}
	if conn.gameData.CommandsEnabled != pk.CommandsEnabled {
		t.Errorf("CommandsEnabled = %v, want %v", conn.gameData.CommandsEnabled, pk.CommandsEnabled)
	}
	if conn.gameData.EducationFeaturesEnabled != pk.EducationFeaturesEnabled {
		t.Errorf("EducationFeaturesEnabled = %v, want %v", conn.gameData.EducationFeaturesEnabled, pk.EducationFeaturesEnabled)
	}
}
