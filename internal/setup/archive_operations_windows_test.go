package setup
import("testing";"os/exec";"path/filepath")
func TestWindowsArchiveOperations(t *testing.T){for _,s:=range []string{"valid","checksum","traversal"}{t.Run(s,func(t *testing.T){c:=exec.Command("powershell.exe","-NoProfile","-ExecutionPolicy","Bypass","-File",filepath.Join("..","..","scripts","tests","archive-operations-harness.ps1"),"-FixtureRoot",t.TempDir(),"-Scenario",s);if b,e:=c.CombinedOutput();e!=nil{t.Fatalf("%v: %s",e,b)}})}}
