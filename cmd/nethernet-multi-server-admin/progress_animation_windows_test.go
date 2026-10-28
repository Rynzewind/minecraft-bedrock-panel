package main

import (
	"github.com/lxn/walk"
	"runtime"
	"testing"
	"time"
)

func TestThemedProgressAnimatesAndStops(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	owner, err := walk.NewMainWindow()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Dispose()
	if err = owner.SetLayout(walk.NewVBoxLayout()); err != nil {
		t.Fatal(err)
	}
	bar, err := walk.NewProgressBar(owner)
	if err != nil {
		t.Fatal(err)
	}
	skin := &minecraftSkin{kind: "progress"}
	mcInstall(bar.Handle(), skin)
	bar.SetMarqueeMode(true)
	owner.Starting().Attach(func() {
		time.AfterFunc(time.Second, func() {
			owner.Synchronize(func() {
				if skin.animationTick < 2 {
					t.Errorf("animation did not advance: %d", skin.animationTick)
				}
				bar.SetMarqueeMode(false)
				tick := skin.animationTick
				time.AfterFunc(250*time.Millisecond, func() {
					owner.Synchronize(func() {
						if skin.animationTick != tick {
							t.Error("animation continued after completion")
						}
						owner.Close()
					})
				})
			})
		})
	})
	owner.Run()
}
