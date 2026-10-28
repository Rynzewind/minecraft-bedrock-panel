package migration

import (
	"context"
	"encoding/json"
	"errors"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeSupervisor struct {
	mu                    sync.Mutex
	states                []supervisor.Status
	failStop, failStart   string
	block                 <-chan struct{}
	stopAfterCancellation bool
}

func (s *fakeSupervisor) Statuses() []supervisor.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]supervisor.Status(nil), s.states...)
}
func (s *fakeSupervisor) Stop(ctx context.Context, id string) error {
	if s.block != nil {
		select {
		case <-ctx.Done():
			if s.stopAfterCancellation {
				s.mu.Lock()
				s.states[0].State = supervisor.Stopping
				s.mu.Unlock()
				go func() {
					time.Sleep(50 * time.Millisecond)
					s.mu.Lock()
					s.states[0].State = supervisor.Stopped
					s.mu.Unlock()
				}()
			}
			return ctx.Err()
		case <-s.block:
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == s.failStop {
		return errors.New("stop failed")
	}
	for i := range s.states {
		if s.states[i].ID == id {
			s.states[i].State = supervisor.Stopped
		}
	}
	return nil
}

func TestCancelDuringStopWaitsForExitBeforeRecovery(t *testing.T) {
	block := make(chan struct{})
	s := &fakeSupervisor{states: []supervisor.Status{{ID: "creative", State: supervisor.Running}}, block: block, stopAfterCancellation: true}
	m := NewManager(fixture(t), s, emptyExport)
	id, e := m.Start(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	for {
		j, _ := m.Status(id)
		if j.Phase == "stopping" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(10 * time.Millisecond)
	m.Cancel(id)
	j := waitJob(t, m, id)
	if j.RestartError != "" || s.Statuses()[0].State != supervisor.Running {
		t.Fatalf("recovery failed: %+v, %+v", j, s.Statuses())
	}
}

func TestInterruptedSnapshotCleansItsOwnStaging(t *testing.T) {
	root := fixture(t)
	output := t.TempDir()
	stage := filepath.Join(output, ".minecraft-snapshot-0123456789abcdef-old")
	if e := os.Mkdir(stage, 0700); e != nil {
		t.Fatal(e)
	}
	partial := filepath.Join(output, ".snapshot-0123456789abcdef.partial")
	os.WriteFile(partial, []byte("partial"), 0600)
	b, _ := json.Marshal(map[string]any{"job_id": "0123456789abcdef", "phase": "copying", "stage_directory": stage, "output_directory": output})
	put(t, root, "MultiServer/migration-job.json", string(b))
	m := NewManager(root, &fakeSupervisor{}, emptyExport)
	j, e := m.Status("")
	if e != nil || !j.Done || j.Phase != "failed" {
		t.Fatal(j, e)
	}
	if _, e = os.Stat(stage); !os.IsNotExist(e) {
		t.Fatal("interrupted staging remains")
	}
	if _, e = os.Stat(partial); !os.IsNotExist(e) {
		t.Fatal("interrupted partial archive remains")
	}
}
func (s *fakeSupervisor) Start(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == s.failStart {
		return errors.New("start failed")
	}
	for i := range s.states {
		if s.states[i].ID == id {
			s.states[i].State = supervisor.Running
		}
	}
	return nil
}
func waitJob(t *testing.T, m *Manager, id string) JobStatus {
	t.Helper()
	end := time.Now().Add(15 * time.Second)
	for time.Now().Before(end) {
		s, e := m.Status(id)
		if e != nil {
			t.Fatal(e)
		}
		if s.Done {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job timed out")
	return JobStatus{}
}
func emptyExport() ([]byte, error) { return []byte(`{"version":1,"accounts":[],"players":[]}`), nil }
func TestStopsBeforeCopyAndStoppedServerStaysStopped(t *testing.T) {
	root := fixture(t)
	s := &fakeSupervisor{states: []supervisor.Status{{ID: "creative", State: supervisor.Running}, {ID: "survival", State: supervisor.Stopped}}}
	m := NewManager(root, s, func() ([]byte, error) {
		if s.Statuses()[0].State != supervisor.Stopped {
			return nil, errors.New("copied live world")
		}
		return emptyExport()
	})
	id, e := m.Start(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	j := waitJob(t, m, id)
	if j.Phase != "complete" || !j.Validated {
		t.Fatalf("job: %+v", j)
	}
	states := s.Statuses()
	if states[0].State != supervisor.Running || states[1].State != supervisor.Stopped {
		t.Fatal(states)
	}
	if _, e = VerifyArchive(context.Background(), j.Path); e != nil {
		t.Fatal(e)
	}
}
func TestStopFailureAbortsAndRecovers(t *testing.T) {
	s := &fakeSupervisor{states: []supervisor.Status{{ID: "creative", State: supervisor.Running}, {ID: "survival", State: supervisor.Running}}, failStop: "survival"}
	m := NewManager(fixture(t), s, emptyExport)
	out := t.TempDir()
	id, e := m.Start(context.Background(), out)
	if e != nil {
		t.Fatal(e)
	}
	j := waitJob(t, m, id)
	if j.Phase != "failed" || j.Path != "" {
		t.Fatal(j)
	}
	if s.Statuses()[0].State != supervisor.Running {
		t.Fatal("not recovered")
	}
	files, _ := filepath.Glob(filepath.Join(out, "*.zip"))
	if len(files) != 0 {
		t.Fatal("published failed archive")
	}
}
func TestCancelRestoresRunningSetAndConcurrentJobRejected(t *testing.T) {
	block := make(chan struct{})
	s := &fakeSupervisor{states: []supervisor.Status{{ID: "creative", State: supervisor.Running}}, block: block}
	m := NewManager(fixture(t), s, emptyExport)
	id, e := m.Start(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.Start(context.Background(), t.TempDir()); e == nil {
		t.Fatal("accepted concurrent job")
	}
	if e = m.Cancel(id); e != nil {
		t.Fatal(e)
	}
	j := waitJob(t, m, id)
	if j.Phase != "cancelled" || s.Statuses()[0].State != supervisor.Running {
		t.Fatal(j)
	}
}
func TestRestartErrorIsReported(t *testing.T) {
	s := &fakeSupervisor{states: []supervisor.Status{{ID: "creative", State: supervisor.Running}}, failStart: "creative"}
	m := NewManager(fixture(t), s, emptyExport)
	id, e := m.Start(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	j := waitJob(t, m, id)
	if j.RestartError == "" || !j.Validated {
		t.Fatal(j)
	}
}
func TestCaptureFailureNeverPublishes(t *testing.T) {
	root := fixture(t)
	s := &fakeSupervisor{states: []supervisor.Status{{ID: "creative", State: supervisor.Running}}}
	m := NewManager(root, s, func() ([]byte, error) { return nil, os.ErrPermission })
	id, e := m.Start(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	j := waitJob(t, m, id)
	if j.Path != "" || j.Phase != "failed" || s.Statuses()[0].State != supervisor.Running {
		t.Fatal(j)
	}
}

func TestInterruptedSnapshotCleansOwnedLocalStaging(t *testing.T) {
	root := fixture(t)
	output := t.TempDir()
	parent := filepath.Join(root, "MultiServer", "backups", "snapshot-staging")
	stage := filepath.Join(parent, ".minecraft-snapshot-0123456789abcdef-local")
	if err := os.MkdirAll(stage, 0700); err != nil {
		t.Fatal(err)
	}
	put(t, root, "MultiServer/migration-job.json", `{"job_id":"0123456789abcdef","phase":"copying","stage_directory":`+mustJSON(t, stage)+`,"output_directory":`+mustJSON(t, output)+`}`)
	m := NewManager(root, &fakeSupervisor{}, emptyExport)
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatal("owned interrupted local staging remains")
	}
	job, _ := m.Status("")
	if !job.Done {
		t.Fatal("interrupted job not recovered")
	}
}
func mustJSON(t *testing.T, value string) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInterruptedLocalCleanupDoesNotRequireDestination(t *testing.T) {
	root := fixture(t)
	parent := filepath.Join(root, "MultiServer", "backups", "snapshot-staging")
	stage := filepath.Join(parent, ".minecraft-snapshot-0123456789abcdef-local")
	if err := os.MkdirAll(stage, 0700); err != nil {
		t.Fatal(err)
	}
	output := root
	put(t, root, "MultiServer/migration-job.json", `{"job_id":"0123456789abcdef","phase":"copying","stage_directory":`+mustJSON(t, stage)+`,"output_directory":`+mustJSON(t, output)+`}`)
	NewManager(root, &fakeSupervisor{}, emptyExport)
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatal("local staging retained because destination is unavailable")
	}
}
