package migration

import (
	"context"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotToCloudDrive(t *testing.T) {
	base := os.Getenv("NN_TEST_CLOUD_ROOT")
	if base == "" {
		t.Skip("requires explicit cloud test directory")
	}
	out, err := os.MkdirTemp(base, "NetherNet-snapshot-test-")
	if err != nil {
		t.Fatal(err)
	}
	absolute, _ := filepath.Abs(base)
	resolved, _ := filepath.Abs(out)
	if !strings.HasPrefix(resolved, absolute+string(filepath.Separator)) || !strings.HasPrefix(filepath.Base(out), "NetherNet-snapshot-test-") {
		t.Fatal("unsafe test cleanup scope")
	}
	defer os.RemoveAll(out)
	t.Logf("Cloud capabilities: free-space check=%v; private-directory ACL=%v", checkSpace(out, 1), privateDirectory(out))
	root := fixture(t)
	s := &fakeSupervisor{states: []supervisor.Status{{ID: "creative", State: supervisor.Stopped}, {ID: "survival", State: supervisor.Stopped}}}
	m := NewManager(root, s, emptyExport)
	id, err := m.Start(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	job := waitJob(t, m, id)
	if job.Error != "" || !job.Validated {
		t.Fatalf("cloud snapshot failed: %+v", job)
	}
	if _, err = VerifyArchive(context.Background(), job.Path); err != nil {
		t.Fatal(err)
	}
}
