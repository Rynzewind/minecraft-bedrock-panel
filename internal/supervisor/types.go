package supervisor

import (
	"context"
	"io"
	"time"

	"nethernet-multi-server/internal/config"
)

type State string

const (
	Unconfigured      State = "unconfigured"
	Stopped           State = "stopped"
	Starting          State = "starting"
	Running           State = "running"
	Stopping          State = "stopping"
	ExternallyRunning State = "externally_running"
	Failed            State = "failed"
)

type Status struct {
	ID, Name       string
	Phase, Backend string
	State          State
	PID            int
	Started        time.Time
	LastError      string
	Pending        bool
	Ready          bool
	Players        int
}

// Progress deliberately excludes pack inventory, which may be locked while
// pending changes are being copied. It remains readable during that work.
type Progress struct {
	Status Status
	Log    string
}
type Process interface {
	PID() int
	Write([]byte) (int, error)
	Wait() error
}
type ProcessFactory interface {
	Start(context.Context, config.Process, io.Writer) (Process, error)
}
type Options struct {
	StopTimeout  time.Duration
	ApplyPending func(context.Context, string) error
	MaxLogBytes  int
}
