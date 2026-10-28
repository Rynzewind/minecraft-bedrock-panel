package fleet

import (
	"testing"
	"time"
)

func TestRecoveryBudgetBoundsCrashLoops(t *testing.T) {
	var r Recovery
	now := time.Now()
	for i := 0; i < 3; i++ {
		if !r.Allow(now.Add(time.Duration(i) * time.Minute)) {
			t.Fatal("retry rejected")
		}
		if r.Allow(now.Add(time.Duration(i)*time.Minute + time.Second)) {
			t.Fatal("retry too soon")
		}
	}
	if r.Allow(now.Add(4 * time.Minute)) {
		t.Fatal("unbounded retry")
	}
	if !r.Allow(now.Add(16 * time.Minute)) {
		t.Fatal("window did not reset")
	}
	r.Reset()
	if !r.Allow(now.Add(16 * time.Minute)) {
		t.Fatal("manual reset failed")
	}
}
