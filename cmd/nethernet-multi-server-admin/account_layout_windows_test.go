package main

import (
	"runtime"
	"testing"
	"time"
)

func TestAccountListHeightGrowsWithRowsAndStopsAtLimit(t *testing.T) {
	if got := accountListHeight(0, 20, 30, 17, 4); got != 71 {
		t.Fatalf("empty list height = %d", got)
	}
	if got := accountListHeight(4, 20, 30, 17, 4); got != 131 {
		t.Fatalf("four accounts height = %d", got)
	}
	if accountListHeight(8, 20, 30, 17, 4) != accountListHeight(80, 20, 30, 17, 4) {
		t.Fatal("large account pool keeps growing instead of scrolling")
	}
}

func TestNativeAccountListSizesFollowModelCounts(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.mw.Dispose()
	a.navigation.SetCurrentIndex(2)
	heights := map[int]int{}
	groups := map[int]int{}
	counts := []int{1, 4, 8, 80, 0}
	var next func(int)
	next = func(index int) {
		if index > 0 {
			count := counts[index-1]
			heights[count] = a.accounts.BoundsPixels().Height
			groups[count] = a.accountPool.BoundsPixels().Height
			if a.accounts.MinSize().Height != a.accounts.MaxSize().Height {
				t.Error("account list retains excess stretch space")
			}
		}
		if index == len(counts) {
			a.mw.Close()
			return
		}
		a.accountModel.set(make([]AccountRow, counts[index]))
		time.AfterFunc(250*time.Millisecond, func() { a.mw.Synchronize(func() { next(index + 1) }) })
	}
	a.mw.Starting().Attach(func() { next(0) })
	a.mw.Run()
	if heights[4] <= heights[1] || heights[8] <= heights[4] || heights[80] != heights[8] || heights[0] > heights[1] {
		t.Fatalf("unexpected native list heights: %v", heights)
	}
	if groups[4] <= groups[1] || groups[80] != groups[8] || groups[0] > groups[1] {
		t.Fatalf("account group did not fit its contents: %v", groups)
	}
}
