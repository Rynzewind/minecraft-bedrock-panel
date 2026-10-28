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
	"os"
	"path/filepath"
	"strings"
	"time"
)

type BackupMetadata struct {
	Kind  string `json:"kind"`
	Owner string `json:"owner"`
	JobID string `json:"job_id"`
}
type retentionRecord struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	JobID  string `json:"job_id"`
}
type retentionLedger struct {
	Version int               `json:"version"`
	Owner   string            `json:"owner"`
	Records []retentionRecord `json:"records"`
}

func hexID(s string, n int) bool { _, err := hex.DecodeString(s); return len(s) == n && err == nil }
func (m *Manager) snapshotOwner() (string, error) {
	path := filepath.Join(m.root, "MultiServer", "installer", "snapshot-owner.json")
	if err := noLinks(path); err != nil {
		return "", err
	}
	var owner string
	b, err := os.ReadFile(path)
	if err == nil {
		if json.Unmarshal(b, &owner) != nil || !hexID(owner, 32) {
			return "", errors.New("invalid snapshot ownership file; repair before enabling cleanup")
		}
		return owner, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return "", err
	}
	owner = hex.EncodeToString(id)
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	b, _ = json.Marshal(owner)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	_, err = f.Write(b)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return owner, err
}
func (m *Manager) retentionPath() string {
	return filepath.Join(m.root, "MultiServer", "backups", "snapshot-retention.json")
}
func writeRetention(path string, ledger retentionLedger) error {
	if err := noLinks(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".retention-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func scheduledRoot(path string) (string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", errors.New("unsafe scheduled snapshot path")
	}
	if err := noLinks(path); err != nil {
		return "", err
	}
	dated := filepath.Dir(path)
	name := filepath.Base(dated)
	stamp, err := time.Parse("2006-01-02_15-04-05", name)
	if err != nil || stamp.Format("2006-01-02_15-04-05") != name || filepath.Base(filepath.Dir(dated)) != "Scheduled" {
		return "", errors.New("cleanup requires an exact dated Scheduled folder")
	}
	return filepath.Dir(dated), nil
}
func archiveDigest(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("archive is not a regular file")
	}
	h := sha256.New()
	if _, err = io.Copy(h, contextReader{ctx, f}); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func verifyScheduled(ctx context.Context, path, owner, jobID string) (string, error) {
	if _, err := scheduledRoot(path); err != nil {
		return "", err
	}
	if !hexID(jobID, 16) || !strings.HasPrefix(filepath.Base(path), "Minecraft-") || !strings.HasSuffix(filepath.Base(path), "-"+jobID+".zip") {
		return "", errors.New("scheduled archive filename does not match its job")
	}
	manifest, err := VerifyArchive(ctx, path)
	if err != nil {
		return "", err
	}
	if manifest.Backup == nil || manifest.Backup.Kind != "scheduled" || manifest.Backup.Owner != owner || manifest.Backup.JobID != jobID {
		return "", errors.New("archive ownership does not match this installation")
	}
	return archiveDigest(ctx, path)
}

// retainScheduled never scans/adopts existing archives. Only the local ledger,
// matching archive provenance and matching bytes authorize an exact file removal.
func (m *Manager) retainScheduled(ctx context.Context, path string) error {
	root, err := scheduledRoot(path)
	if err != nil {
		return err
	}
	owner, err := m.snapshotOwner()
	if err != nil {
		return err
	}
	ledger := retentionLedger{Version: 1, Owner: owner}
	if err = noLinks(m.retentionPath()); err != nil {
		return err
	}
	b, err := os.ReadFile(m.retentionPath())
	if err == nil {
		if json.Unmarshal(b, &ledger) != nil || ledger.Version != 1 || ledger.Owner != owner {
			return errors.New("retention ledger is invalid; no backups were removed")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	manifest, err := VerifyArchive(ctx, path)
	if err != nil {
		return err
	}
	if manifest.Backup == nil {
		return errors.New("missing backup provenance")
	}
	digest, err := verifyScheduled(ctx, path, owner, manifest.Backup.JobID)
	if err != nil {
		return err
	}
	for _, r := range ledger.Records {
		if strings.EqualFold(r.Path, path) {
			return errors.New("snapshot already registered; no cleanup attempted")
		}
	}
	ledger.Records = append(ledger.Records, retentionRecord{path, digest, manifest.Backup.JobID})
	if err = writeRetention(m.retentionPath(), ledger); err != nil {
		return err
	}
	var candidates []retentionRecord
	seen := map[string]bool{}
	for _, r := range ledger.Records {
		candidateRoot, e := scheduledRoot(r.Path)
		if e != nil {
			return fmt.Errorf("unsafe tracked backup; no cleanup: %w", e)
		}
		key := strings.ToLower(r.Path)
		if seen[key] {
			return errors.New("duplicate retention record; no cleanup")
		}
		seen[key] = true
		if !strings.EqualFold(candidateRoot, root) {
			continue
		}
		got, e := verifyScheduled(ctx, r.Path, owner, r.JobID)
		if e != nil {
			return fmt.Errorf("tracked backup could not be verified; no cleanup: %w", e)
		}
		if got != r.SHA256 {
			return errors.New("tracked backup changed; no backups were removed")
		}
		candidates = append(candidates, r)
	}
	for len(candidates) > 7 {
		old := candidates[0]
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = noLinks(old.Path); err != nil {
			return err
		}
		if err = os.Remove(old.Path); err != nil {
			return err
		}
		// Never recurse: unrelated contents keep their folder and are untouched.
		_ = os.Remove(filepath.Dir(old.Path))
		for i, r := range ledger.Records {
			if r.Path == old.Path {
				ledger.Records = append(ledger.Records[:i], ledger.Records[i+1:]...)
				break
			}
		}
		if err = writeRetention(m.retentionPath(), ledger); err != nil {
			return err
		}
		candidates = candidates[1:]
	}
	return nil
}
