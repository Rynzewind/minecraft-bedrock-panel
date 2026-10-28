package migration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"nethernet-multi-server/internal/config"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Supervisor interface {
	Statuses() []supervisor.Status
	Start(context.Context, string) error
	Stop(context.Context, string) error
}
type JobStatus struct {
	Warning         string    `json:"warning,omitempty"`
	WarningSeconds  int       `json:"warning_seconds,omitempty"`
	ID              string    `json:"job_id"`
	Phase           string    `json:"phase"`
	Progress        int       `json:"progress"`
	Done            bool      `json:"done"`
	Validated       bool      `json:"validated"`
	Path            string    `json:"path,omitempty"`
	Error           string    `json:"error,omitempty"`
	RestartError    string    `json:"restart_error,omitempty"`
	RunningIDs      []string  `json:"running_ids,omitempty"`
	CreatedUTC      time.Time `json:"created_utc"`
	StageDirectory  string    `json:"stage_directory,omitempty"`
	OutputDirectory string    `json:"output_directory,omitempty"`
}
type Manager struct {
	warningWait func(context.Context, time.Duration) error
	mu          sync.Mutex
	root        string
	supervisor  Supervisor
	export      func() ([]byte, error)
	job         JobStatus
	cancel      context.CancelFunc
	busy        bool
}

func NewManager(root string, s Supervisor, export func() ([]byte, error)) *Manager {
	root, _ = filepath.Abs(root)
	m := &Manager{root: root, supervisor: s, export: export}
	if b, e := os.ReadFile(m.journalPath()); e == nil {
		if json.Unmarshal(b, &m.job) == nil && !m.job.Done {
			m.job.Done = true
			m.job.Phase = "failed"
			m.job.Error = "Snapshot interrupted by helper shutdown. Check server states before retrying."
			m.cleanInterrupted()
			m.saveLocked()
		}
	}
	return m
}

func (m *Manager) cleanInterrupted() {
	j := &m.job
	if len(j.ID) != 16 {
		return
	}
	if _, e := hex.DecodeString(j.ID); e != nil {
		return
	}
	stage := filepath.Clean(j.StageDirectory)
	ownedStage := strings.HasPrefix(filepath.Base(stage), ".minecraft-snapshot-"+j.ID+"-")
	// Local ownership is independent of the cloud drive being mounted/valid.
	if filepath.Dir(stage) == m.stageParent() && ownedStage && noLinks(stage) == nil {
		_ = os.RemoveAll(stage)
	}
	output, err := filepath.Abs(j.OutputDirectory)
	if err != nil || j.OutputDirectory == "" || inside(m.root, output) || inside(output, m.root) || noLinks(output) != nil {
		j.StageDirectory = ""
		return
	}
	if filepath.Dir(stage) == output && ownedStage && noLinks(stage) == nil {
		_ = os.RemoveAll(stage)
	}
	partial := filepath.Join(output, ".snapshot-"+j.ID+".partial")
	if noLinks(partial) == nil {
		_ = os.Remove(partial)
	}
	j.StageDirectory = ""
}
func (m *Manager) stageParent() string {
	return filepath.Join(m.root, "MultiServer", "backups", "snapshot-staging")
}
func (m *Manager) journalPath() string {
	return filepath.Join(m.root, "MultiServer", "migration-job.json")
}
func (m *Manager) saveLocked() {
	b, e := json.MarshalIndent(m.job, "", "  ")
	if e != nil {
		return
	}
	tmp := m.journalPath() + ".tmp"
	if os.WriteFile(tmp, b, 0600) == nil {
		_ = os.Rename(tmp, m.journalPath())
	}
}
func (m *Manager) Busy() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.busy }
func (m *Manager) Status(id string) (JobStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job.ID == "" || id != "" && id != m.job.ID {
		return JobStatus{}, errors.New("snapshot job not found")
	}
	s := m.job
	s.RunningIDs = append([]string(nil), s.RunningIDs...)
	return s, nil
}
func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id != m.job.ID || !m.busy {
		return errors.New("no active matching snapshot")
	}
	m.cancel()
	return nil
}
func (m *Manager) Start(parent context.Context, output string) (string, error) {
	return m.StartWithOptions(parent, output, SnapshotOptions{IncludeAddons: true})
}
func (m *Manager) StartWithOptions(parent context.Context, output string, options SnapshotOptions) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy {
		return "", errors.New("another snapshot is already running")
	}
	if err := parent.Err(); err != nil {
		return "", err
	}
	if options.Kind == "" {
		options.Kind = "manual"
	}
	if options.Kind != "manual" && options.Kind != "scheduled" {
		return "", errors.New("invalid snapshot kind")
	}
	if options.Kind == "scheduled" {
		if _, err := scheduledRoot(output + string(filepath.Separator) + "placeholder.zip"); err != nil {
			return "", err
		}
		options.IncludeAddons = true
	}
	if !filepath.IsAbs(output) {
		return "", errors.New("choose an absolute output folder")
	}
	if inside(m.root, output) || inside(output, m.root) {
		return "", errors.New("snapshot output must be outside the server folder")
	}
	if err := noLinks(output); err != nil {
		return "", err
	}
	for _, s := range m.supervisor.Statuses() {
		switch s.State {
		case supervisor.Running, supervisor.Stopped, supervisor.Failed:
		default:
			return "", fmt.Errorf("server %s must be managed and stable before capture", s.Name)
		}
	}
	b := make([]byte, 8)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	id := hex.EncodeToString(b)
	// A panel disconnect must not cancel the job. Only explicit cancellation does.
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.busy = true
	m.job = JobStatus{ID: id, Phase: "preflight", CreatedUTC: time.Now().UTC(), OutputDirectory: filepath.Clean(output)}
	m.saveLocked()
	go m.capture(ctx, output, options)
	return id, nil
}
func (m *Manager) phase(p string, n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job.Phase == p && m.job.Progress == n {
		return
	}
	m.job.Phase = p
	m.job.Progress = n
	m.saveLocked()
}
func (m *Manager) capture(ctx context.Context, output string, options SnapshotOptions) {
	includeAddons := options.IncludeAddons
	var stage, partial string
	var captureErr, restartErr error
	var running []string
	recovered := false
	recoverServers := func() {
		if recovered {
			return
		}
		recovered = true
		if len(running) > 0 {
			m.phase("restarting", 65)
		}
		recovery, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		for _, id := range running {
			// Cancellation can arrive after the stop command but before process exit.
			// Wait for that exit using the independent recovery budget before restart.
			waiting := true
			for waiting && recovery.Err() == nil {
				waiting = false
				for _, s := range m.supervisor.Statuses() {
					if s.ID == id && s.State == supervisor.Stopping {
						waiting = true
						break
					}
				}
				if waiting {
					select {
					case <-recovery.Done():
					case <-time.After(25 * time.Millisecond):
					}
				}
			}
			for _, s := range m.supervisor.Statuses() {
				if s.ID == id {
					if s.State == supervisor.Stopped || s.State == supervisor.Failed {
						if e := m.supervisor.Start(recovery, id); e != nil {
							restartErr = errors.Join(restartErr, fmt.Errorf("%s: %w", id, e))
						}
					} else if s.State != supervisor.Running {
						restartErr = errors.Join(restartErr, fmt.Errorf("%s is %s; check and start manually", id, s.State))
					}
				}
			}
		}
	}
	defer func() {
		recoverServers()
		if stage != "" {
			_ = os.RemoveAll(stage)
		}
		if partial != "" {
			_ = os.Remove(partial)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		m.busy = false
		m.cancel()
		m.job.Done = true
		m.job.StageDirectory = ""
		if captureErr != nil {
			m.job.Error = captureErr.Error()
			m.job.Phase = "failed"
			if errors.Is(captureErr, context.Canceled) {
				m.job.Phase = "cancelled"
			}
		} else {
			m.job.Phase = "complete"
			m.job.Progress = 100
		}
		if restartErr != nil {
			m.job.RestartError = restartErr.Error()
		}
		m.saveLocked()
	}()
	// Enumerate for free-space sizing before interruption, then enumerate again after stop.
	owner, e := m.snapshotOwner()
	if e != nil {
		captureErr = e
		return
	}
	files, e := inventoryForSnapshotSizing(m.root, output, includeAddons)
	if e != nil {
		captureErr = e
		return
	}
	if e = os.MkdirAll(output, 0700); e != nil {
		captureErr = e
		return
	}
	var size uint64
	for _, f := range files {
		size += uint64(f.Size)
	}
	if e = checkSpace(output, size+(256<<20)); e != nil {
		captureErr = fmt.Errorf("snapshot destination free space: %w", e)
		return
	}
	// Cloud-backed filesystems may reject Windows protected ACLs. Keep raw
	// world/identity staging on the installation volume, never relax privacy.
	stageParent := m.stageParent()
	if e = noLinks(stageParent); e != nil {
		captureErr = fmt.Errorf("local snapshot staging: %w", e)
		return
	}
	if e = os.MkdirAll(stageParent, 0700); e != nil {
		captureErr = fmt.Errorf("create local snapshot staging: %w", e)
		return
	}
	localRequired := size*2 + (256 << 20)
	if e = checkSpace(stageParent, localRequired); e != nil {
		captureErr = fmt.Errorf("local snapshot staging free space: %w", e)
		return
	}
	stage, e = os.MkdirTemp(stageParent, ".minecraft-snapshot-"+m.job.ID+"-")
	if e != nil {
		captureErr = fmt.Errorf("create local snapshot staging: %w", e)
		return
	}
	if e = privateDirectory(stage); e != nil {
		captureErr = fmt.Errorf("protect local snapshot staging: %w", e)
		return
	}
	m.mu.Lock()
	m.job.StageDirectory = stage
	m.saveLocked()
	m.mu.Unlock()
	if options.Kind == "scheduled" {
		if e = m.warnScheduled(ctx); e != nil {
			captureErr = e
			return
		}
	}
	m.phase("stopping", 10)
	for _, s := range m.supervisor.Statuses() {
		if s.State == supervisor.Running {
			running = append(running, s.ID)
		} else if s.State != supervisor.Stopped && s.State != supervisor.Failed {
			captureErr = errors.New("server state changed before capture")
			return
		}
	}
	m.mu.Lock()
	m.job.RunningIDs = append([]string(nil), running...)
	m.saveLocked()
	m.mu.Unlock()
	for _, id := range running {
		if e = m.supervisor.Stop(ctx, id); e != nil {
			captureErr = e
			return
		}
	}
	for _, s := range m.supervisor.Statuses() {
		if s.State != supervisor.Stopped && s.State != supervisor.Failed {
			captureErr = errors.New("server exit was not confirmed")
			return
		}
	}
	m.phase("copying", 20)
	files, e = inventoryForSnapshotSizing(m.root, output, includeAddons)
	if e != nil {
		captureErr = e
		return
	}
	for i, f := range files {
		var capturedEntry FileEntry
		if capturedEntry, e = copyFile(ctx, filepath.Join(m.root, filepath.FromSlash(f.Path)), filepath.Join(stage, filepath.FromSlash(f.Path))); e != nil {
			captureErr = e
			return
		}
		capturedEntry.Path = f.Path
		files[i] = capturedEntry
		m.phase("copying", 20+40*(i+1)/len(files))
	}
	metadata, e := m.export()
	if e != nil {
		captureErr = e
		return
	}
	if _, e = ImportAccounts(metadata); e != nil {
		captureErr = e
		return
	}
	if e = os.WriteFile(filepath.Join(stage, "accounts-migration.json"), metadata, 0600); e != nil {
		captureErr = e
		return
	}
	accountFile, e := fileEntry(stage, filepath.Join(stage, "accounts-migration.json"))
	if e != nil {
		captureErr = e
		return
	}
	files = append(files, accountFile)
	captured := time.Now().UTC()
	recoverServers()
	if e = ctx.Err(); e != nil {
		captureErr = e
		return
	}
	cfg, e := config.Load(filepath.Join(stage, "MultiServer", "config.json"))
	if e != nil {
		captureErr = e
		return
	}
	components := []Component{{"NetherNet", "migration-v1; Xbox LAN answer fix; backend login 90s"}}
	for _, s := range cfg.Servers {
		components = append(components, Component{s.ID, s.Version})
	}
	manifest := Manifest{FormatVersion: FormatVersion, CreatedUTC: captured, SourceRoot: m.root, Files: files, Components: components, AddonLibraryIncluded: &includeAddons}
	manifest.Backup = &BackupMetadata{Kind: options.Kind, Owner: owner, JobID: m.job.ID}
	m.phase("compressing", 70)
	localArchive := filepath.Join(stage, ".snapshot-"+m.job.ID+".partial")
	if e = WriteArchive(ctx, stage, localArchive, manifest); e != nil {
		captureErr = fmt.Errorf("create local snapshot archive: %w", e)
		return
	}
	m.phase("verifying", 90)
	if _, e = VerifyArchive(ctx, localArchive); e != nil {
		captureErr = fmt.Errorf("verify local snapshot archive: %w", e)
		return
	}
	partial = filepath.Join(output, ".snapshot-"+m.job.ID+".partial")
	m.phase("publishing", 95)
	if strings.EqualFold(filepath.VolumeName(stageParent), filepath.VolumeName(output)) {
		e = os.Rename(localArchive, partial)
	} else {
		e = copyVerifiedArchive(ctx, localArchive, partial, func(done, total int64) {
			if total > 0 {
				m.phase("publishing", 95+int(4*done/total))
			}
		})
	}
	if e != nil {
		captureErr = fmt.Errorf("save verified snapshot to destination: %w", e)
		return
	}
	final := filepath.Join(output, "Minecraft-"+captured.Format("20060102-150405")+"-"+m.job.ID+".zip")
	if e = os.Rename(partial, final); e != nil {
		captureErr = e
		return
	}
	partial = ""
	m.mu.Lock()
	m.job.Path = final
	m.job.Validated = true
	m.mu.Unlock()
	if options.Kind == "scheduled" && restartErr == nil {
		m.phase("checking scheduled retention", 99)
		if err := m.retainScheduled(ctx, final); err != nil {
			m.warning("Scheduled backup saved; cleanup needs attention: " + err.Error())
		}
	}
}
func copyFile(ctx context.Context, src, dst string) (entry FileEntry, err error) {
	info, e := os.Lstat(src)
	if e != nil {
		return entry, e
	}
	if !info.Mode().IsRegular() {
		return entry, errors.New("snapshot source is no longer a regular file")
	}
	if err = os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return entry, err
	}
	s, err := os.Open(src)
	if err != nil {
		return entry, err
	}
	defer s.Close()
	d, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return entry, err
	}
	defer func() {
		e := d.Close()
		if err == nil {
			err = e
		}
	}()
	h := sha256.New()
	entry.Size, err = io.Copy(io.MultiWriter(d, h), contextReader{ctx, s})
	entry.SHA256 = hex.EncodeToString(h.Sum(nil))
	return entry, err
}
