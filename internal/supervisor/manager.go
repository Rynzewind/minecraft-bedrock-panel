package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"nethernet-multi-server/internal/config"
)

type managed struct {
	announcementPending bool
	cfg                 config.Server
	status              Status
	process             Process
	exited              chan error
	log                 *boundedLog
}

type boundedLog struct {
	mu    sync.Mutex
	b     []byte
	max   int
	ready bool
}

func (b *boundedLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.b = append(b.b, p...)
	if strings.Contains(string(b.b), "Server started.") {
		b.ready = true
	}
	if len(b.b) > b.max {
		b.b = append([]byte(nil), b.b[len(b.b)-b.max:]...)
	}
	return len(p), nil
}
func (b *boundedLog) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(append([]byte(nil), b.b...))
}

type Manager struct {
	mu      sync.Mutex
	servers map[string]*managed
	factory ProcessFactory
	opts    Options
}

func New(servers []config.Server, factory ProcessFactory, opts Options) *Manager {
	if opts.StopTimeout <= 0 {
		opts.StopTimeout = 30 * time.Second
	}
	if opts.MaxLogBytes <= 0 {
		opts.MaxLogBytes = 256 << 10
	}
	m := &Manager{servers: map[string]*managed{}, factory: factory, opts: opts}
	for _, s := range servers {
		id := s.ID
		if id == "" {
			id = s.Name
		}
		state := Stopped
		if s.Process == nil {
			state = Unconfigured
		}
		m.servers[id] = &managed{cfg: s, status: Status{ID: id, Name: s.Name, State: state, Backend: s.Backend}, log: &boundedLog{max: opts.MaxLogBytes}}
	}
	return m
}

func (m *Manager) log(id string) io.Writer {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.servers[id]; s != nil {
		return s.log
	}
	return io.Discard
}
func (m *Manager) RecentLog(id string) string {
	m.mu.Lock()
	s := m.servers[id]
	if s == nil {
		m.mu.Unlock()
		return ""
	}
	log := s.log
	m.mu.Unlock()
	return log.String()
}
func (m *Manager) MarkExternallyRunning(id string, pid int) error {
	return m.observeExternal(id, pid)
}
func (m *Manager) observeExternal(id string, pid int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.servers[id]
	if s == nil {
		return errors.New("server not found")
	}
	if pid == 0 && s.status.State == ExternallyRunning {
		s.status.State = Stopped
		s.status.PID = 0
		return nil
	}
	if pid == 0 {
		return nil
	}
	if s.status.State != Stopped && s.status.State != ExternallyRunning {
		return fmt.Errorf("server %s is %s", id, s.status.State)
	}
	s.status.State = ExternallyRunning
	s.status.PID = pid
	return nil
}
func (m *Manager) Status(id string) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.servers[id]; s != nil {
		result := s.status
		s.log.mu.Lock()
		result.Ready = s.log.ready
		s.log.mu.Unlock()
		return result
	}
	return Status{ID: id, State: Unconfigured}
}
func (m *Manager) Statuses() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Status, 0, len(m.servers))
	for _, s := range m.servers {
		result := s.status
		s.log.mu.Lock()
		result.Ready = s.log.ready
		s.log.mu.Unlock()
		out = append(out, result)
	}
	return out
}
func (m *Manager) Start(ctx context.Context, id string) error {
	m.mu.Lock()
	s := m.servers[id]
	if s == nil {
		m.mu.Unlock()
		return errors.New("server not found")
	}
	if s.status.State != Stopped && !(s.status.State == Failed && s.process == nil && s.status.PID == 0) {
		state := s.status.State
		m.mu.Unlock()
		return fmt.Errorf("server %s is %s", id, state)
	}
	s.status.State = Starting
	s.status.Phase = "applying_changes"
	m.mu.Unlock()
	if m.opts.ApplyPending != nil {
		if err := m.opts.ApplyPending(ctx, id); err != nil {
			m.fail(s, err)
			return err
		}
	}
	m.mu.Lock()
	s.log = &boundedLog{max: m.opts.MaxLogBytes}
	s.status.Phase = "launching"
	m.mu.Unlock()
	p, err := m.factory.Start(ctx, *s.cfg.Process, m.log(id))
	if err != nil {
		m.fail(s, err)
		return err
	}
	m.mu.Lock()
	s.process = p
	s.announcementPending = false
	s.exited = make(chan error, 1)
	s.status.State = Running
	s.status.Phase = "running"
	s.status.PID = p.PID()
	s.status.Started = time.Now()
	s.status.LastError = ""
	exited := s.exited
	m.mu.Unlock()
	go func() {
		err := p.Wait()
		m.mu.Lock()
		if s.process == p {
			s.process = nil
			s.status.PID = 0
			if err != nil {
				s.status.State = Failed
				s.status.Phase = "failed"
				s.status.LastError = err.Error()
			} else {
				s.status.State = Stopped
				s.status.Phase = "stopped"
			}
			exited <- err
			close(exited)
		}
		m.mu.Unlock()
	}()
	return nil
}
func (m *Manager) fail(s *managed, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.status.State = Failed
	s.status.Phase = "failed"
	s.status.LastError = err.Error()
}
func (m *Manager) Stop(ctx context.Context, id string) error {
	m.mu.Lock()
	s := m.servers[id]
	if s == nil {
		m.mu.Unlock()
		return errors.New("server not found")
	}
	if s.status.State != Running {
		state := s.status.State
		m.mu.Unlock()
		return fmt.Errorf("server %s is %s", id, state)
	}
	if s.announcementPending {
		m.mu.Unlock()
		return errors.New("server console is unresponsive; graceful stop was not sent")
	}
	p, exited := s.process, s.exited
	s.status.State = Stopping
	s.status.Phase = "stopping"
	m.mu.Unlock()
	if m.opts.BeforeStop != nil {
		m.opts.BeforeStop(id)
	}
	if _, err := p.Write([]byte("stop\r\n")); err != nil {
		return err
	}
	timer := time.NewTimer(m.opts.StopTimeout)
	defer timer.Stop()
	select {
	case err := <-exited:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		m.mu.Lock()
		if s.status.State == Stopping {
			s.status.State = Running
			s.status.Phase = "running"
		}
		m.mu.Unlock()
		return errors.New("graceful stop timed out")
	}
}
func (m *Manager) Restart(ctx context.Context, id string) error {
	if err := m.Stop(ctx, id); err != nil {
		return err
	}
	return m.Start(ctx, id)
}
