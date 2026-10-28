package proxy

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestBackendLoginCanFinishAfterThirtySeconds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := backendDialContext(context.Background())
		defer cancel()
		time.Sleep(35 * time.Second)
		if err := ctx.Err(); err != nil {
			t.Fatalf("slow world login was interrupted before completion: %v", err)
		}
		time.Sleep(56 * time.Second)
		if ctx.Err() != context.DeadlineExceeded {
			t.Fatal("unresponsive backend was not bounded by the login deadline")
		}
	})
}

func TestBackendLoginStopsWhenSessionIsCancelled(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, cancel := backendDialContext(parent)
	defer cancel()
	cancelParent()
	if ctx.Err() != context.Canceled {
		t.Fatal("backend login continued after its session was cancelled")
	}
}

func TestCameraFieldOfViewRecognisesFOVInstruction(t *testing.T) {
	pk := &packet.CameraInstruction{FieldOfView: protocol.Option(protocol.CameraInstructionFieldOfView{FieldOfView: 37.5})}
	got, ok := cameraFieldOfView(pk)
	if !ok || got != 37.5 {
		t.Fatalf("cameraFieldOfView() = %v, %v; want 37.5, true", got, ok)
	}
}

func TestCameraFieldOfViewIgnoresInstructionWithoutFOV(t *testing.T) {
	if _, ok := cameraFieldOfView(&packet.CameraInstruction{}); ok {
		t.Fatal("instruction without FOV reported as FOV instruction")
	}
}
