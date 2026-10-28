package online

import (
	"golang.org/x/sys/windows"
	"nethernet-multi-server/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveProtectsDirectoryAndNewIsReadOnly(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Online")
	m, e := New(filepath.Join(dir, "settings.json"), []config.Server{{ID: "a"}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(dir); !os.IsNotExist(e) {
		t.Fatal("New created directory")
	}
	if e = m.Save(m.Settings()); e != nil {
		t.Fatal(e)
	}
	sd, e := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		t.Fatal(e)
	}
	u, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		t.Fatal(e)
	}
	sddl := sd.String()
	for _, want := range []string{"D:P", ";;;SY)", ";;;BA)", ";;;" + u.User.Sid.String() + ")"} {
		if !strings.Contains(sddl, want) {
			t.Fatalf("missing private ACL %s: %s", want, sddl)
		}
	}
	if strings.Contains(sddl, ";;;WD)") || strings.Contains(sddl, ";;;BU)") {
		t.Fatal("broad ACL", sddl)
	}
}
