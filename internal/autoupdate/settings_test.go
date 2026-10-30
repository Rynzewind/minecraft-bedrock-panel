package autoupdate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIndependentOptionsPersistAndDefaultOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "automatic.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Get().Servers || s.Get().Addons {
		t.Fatal("unexpected opt-in")
	}
	yes, no := true, false
	s.Set(&yes, nil)
	s.Set(nil, &yes)
	s.Set(&no, nil)
	loaded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Get().Servers || !loaded.Get().Addons {
		t.Fatal("switches did not persist independently")
	}
	if err = loaded.Record("checked"); err != nil {
		t.Fatal(err)
	}
	loaded, _ = Open(path)
	if loaded.Get().NextCheck.IsZero() {
		t.Fatal("schedule not persisted")
	}
}
func TestInvalidAutomaticSettingsFailClosed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(p, []byte("not-json"), 0600)
	if _, err := Open(p); err == nil {
		t.Fatal("corrupt settings accepted")
	}
}

func TestAutomaticChecksWaitForIdleWindowAndNextCheck(t *testing.T) {
	now := time.Now()
	s := Settings{Servers: true}
	if Due(now, now.Add(-4*time.Minute), s) {
		t.Fatal("idle grace period ignored")
	}
	if !Due(now, now.Add(-5*time.Minute), s) {
		t.Fatal("due idle update was skipped")
	}
	s.NextCheck = now.Add(time.Minute)
	if Due(now, now.Add(-time.Hour), s) {
		t.Fatal("check interval ignored")
	}
	if Due(now, now.Add(-time.Hour), Settings{}) {
		t.Fatal("disabled updater ran")
	}
}
