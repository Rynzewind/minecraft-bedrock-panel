package setup

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadableConfigurationScriptWorkflows(t *testing.T) {
	for _, scenario := range []string{"success", "public"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			for _, p := range []string{"MultiServer/NetherNetMultiServer.exe", "Creative/bedrock_server.exe", "Survival/bedrock_server.exe"} {
				p = filepath.Join(root, filepath.FromSlash(p))
				os.MkdirAll(filepath.Dir(p), 0700)
				if e := os.WriteFile(p, []byte("fixture"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			cfg := map[string]any{"interface_ip": "10.42.8.9", "servers": []any{map[string]any{"id": "creative", "process": map[string]string{"executable": filepath.Join(root, "Creative", "bedrock_server.exe")}}, map[string]any{"id": "survival", "process": map[string]string{"executable": filepath.Join(root, "Survival", "bedrock_server.exe")}}}}
			write := func(path string, v any) {
				t.Helper()
				b, e := json.Marshal(v)
				if e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(path, b, 0600); e != nil {
					t.Fatal(e)
				}
			}
			write(filepath.Join(root, "MultiServer", "config.json"), cfg)
			write(filepath.Join(root, "plan.json"), Plan{Root: root, UserSID: "S-1-5-18", Startup: "logon", Group: "NetherNet-Migration-012345abcdef", FirewallPrograms: []string{`C:\Windows\System32\calc.exe`}, Network: &NetworkPlan{InterfaceIndex: 9, PrimaryIP: "10.42.8.9", DiscoveryPort: 7551, Servers: []ServerNetwork{{ID: "creative", IP: "10.42.9.246", TCPPort: 19134, UDPRange: "49152-49200"}, {ID: "survival", IP: "10.42.9.247", TCPPort: 19136, UDPRange: "49201-49249"}}}})
			command := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join("..", "..", "scripts", "tests", "configuration-harness.ps1"), "-FixtureRoot", root, "-Scenario", scenario)
			output, err := command.CombinedOutput()
			var result struct {
				Success bool   `json:"success"`
				Error   string `json:"error"`
			}
			b, e := os.ReadFile(filepath.Join(root, "setup-result.json"))
			if e != nil {
				t.Fatalf("script result missing: %v %s", err, output)
			}
			if e = json.Unmarshal(b, &result); e != nil {
				t.Fatal(e)
			}
			if scenario == "public" {
				if err == nil || result.Success {
					t.Fatal("public LAN should remain gated")
				}
				if _, e = os.Stat(filepath.Join(root, "migration-activation-required")); e != nil {
					t.Fatal("failed setup activated")
				}
				if _, e = os.Stat(filepath.Join(root, "enabled.txt")); !os.IsNotExist(e) {
					t.Fatal("failed setup enabled startup")
				}
				return
			}
			if err != nil || !result.Success {
				t.Fatalf("script failed: %v %s %s", err, output, result.Error)
			}
			b, e = os.ReadFile(filepath.Join(root, "firewall.jsonl"))
			if e != nil {
				t.Fatal(e)
			}
			lines := strings.Split(strings.TrimSpace(string(b)), "\n")
			if len(lines) != 5 {
				t.Fatalf("expected 5 scoped firewall rules, got %d", len(lines))
			}
			for _, line := range lines {
				var rule struct{ Program, Protocol, Profile, RemoteAddress, LocalAddress string }
				if e = json.Unmarshal([]byte(line), &rule); e != nil {
					t.Fatal(e)
				}
				if filepath.Base(rule.Program) == "NetherNetMultiServer.exe" && rule.Protocol == "UDP" && rule.LocalAddress != "Any" {
					t.Fatalf("discovery firewall must accept broadcast destinations on the selected interface: %+v", rule)
				}
				if !strings.HasPrefix(rule.Program, root+string(filepath.Separator)) || (rule.Protocol != "UDP" && rule.Protocol != "TCP") || rule.Profile != "Private" || rule.RemoteAddress != "LocalSubnet" {
					t.Fatalf("unsafe firewall scope: %+v", rule)
				}
			}
			b, e = os.ReadFile(filepath.Join(root, "task.json"))
			if e != nil {
				t.Fatal(e)
			}
			var task struct {
				Principal struct{ UserId, RunLevel string }
				Settings  struct{ Disabled bool }
			}
			if e = json.Unmarshal(b, &task); e != nil {
				t.Fatal(e)
			}
			if task.Principal.UserId != `NT AUTHORITY\SYSTEM` || task.Principal.RunLevel != "Limited" || !task.Settings.Disabled {
				t.Fatalf("startup principal/settings wrong: %+v", task)
			}
			if _, e = os.Stat(filepath.Join(root, "migration-activation-required")); !os.IsNotExist(e) {
				t.Fatal("successful setup still gated")
			}
		})
	}
}
