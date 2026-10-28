package fleet

import (
	"context"
	"errors"
	"fmt"
	"nethernet-multi-server/internal/supervisor"
	"sort"
)

type Client interface {
	Call(context.Context, string, any, any) error
}
type Host interface {
	Ensure(context.Context) error
	Running() (bool, error)
	WaitStopped(context.Context) error
}
type Engine struct {
	Client Client
	Host   Host
	Ready  func(context.Context, supervisor.Status) error
}

func (e *Engine) Run(ctx context.Context, action string, notify func(string)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if notify == nil {
		notify = func(string) {}
	}
	switch action {
	case "start-all":
		return e.start(ctx, notify, false)
	case "recover":
		running, err := e.Host.Running()
		if err != nil {
			return err
		}
		return e.start(ctx, notify, running)
	case "stop-all":
		return e.stop(ctx, notify)
	case "restart-all":
		if err := e.stop(ctx, notify); err != nil {
			return err
		}
		return e.start(ctx, notify, false)
	default:
		return errors.New("unknown fleet action")
	}
}
func (e *Engine) statuses(ctx context.Context) ([]supervisor.Status, error) {
	var states []supervisor.Status
	err := e.Client.Call(ctx, "servers.list", struct{}{}, &states)
	sort.Slice(states, func(i, j int) bool { return states[i].ID < states[j].ID })
	return states, err
}
func (e *Engine) start(ctx context.Context, notify func(string), recoverOnly bool) error {
	notify("Starting discovery helper and waiting for network availability")
	if err := e.Host.Ensure(ctx); err != nil {
		return err
	}
	states, err := e.statuses(ctx)
	if err != nil {
		return err
	}
	if len(states) == 0 {
		return errors.New("no managed servers are configured")
	}
	var failures []error
	for _, s := range states {
		if recoverOnly && s.State != supervisor.Failed {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		switch s.State {
		case supervisor.Stopped, supervisor.Failed:
			notify("Starting " + s.Name)
			if err := e.Client.Call(ctx, "server.start", map[string]string{"server_id": s.ID}, nil); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", s.Name, err))
				continue
			}
		case supervisor.Running:
			notify("Checking already running " + s.Name)
		default:
			failures = append(failures, fmt.Errorf("%s is %s; cannot safely take control", s.Name, s.State))
			continue
		}
		if e.Ready != nil {
			notify("Waiting for full startup and backend readiness: " + s.Name)
			if err := e.Ready(ctx, s); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", s.Name, err))
			}
		}
	}
	return errors.Join(failures...)
}
func (e *Engine) stop(ctx context.Context, notify func(string)) error {
	running, err := e.Host.Running()
	if err != nil {
		return err
	}
	if !running {
		if verifier, ok := e.Host.(interface{ VerifyStopped() error }); ok {
			if err := verifier.VerifyStopped(); err != nil {
				return err
			}
		}
		notify("Servers and helper are already stopped")
		return nil
	}
	states, err := e.statuses(ctx)
	if err != nil {
		return err
	}
	for _, s := range states {
		if s.State != supervisor.Running && s.State != supervisor.Stopped && s.State != supervisor.Failed {
			return fmt.Errorf("%s is %s; wait or stop it from its original console", s.Name, s.State)
		}
	}
	for _, s := range states {
		if s.State == supervisor.Running {
			notify("Saving world and stopping " + s.Name)
			if err := e.Client.Call(ctx, "server.stop", map[string]string{"server_id": s.ID}, nil); err != nil {
				return fmt.Errorf("%s: %w", s.Name, err)
			}
		}
	}
	notify("Stopping discovery helper")
	if err := e.Client.Call(ctx, "system.shutdown", struct{}{}, nil); err != nil {
		return err
	}
	return e.Host.WaitStopped(ctx)
}
