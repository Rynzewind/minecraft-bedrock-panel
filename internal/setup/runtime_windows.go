package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type RuntimeCheck struct {
	Check  string `json:"check"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func StartRuntime(ctx context.Context, root string, ids []string) error {
	if _, e := os.Stat(filepath.Join(root, "installation-retired")); e == nil {
		return fmt.Errorf("installation is retired")
	}
	processes, e := InstallationProcesses(root)
	if e != nil {
		return e
	}
	helper := false
	for _, p := range processes {
		if strings.EqualFold(p.Path, filepath.Join(root, "MultiServer", "NetherNetMultiServer.exe")) {
			helper = true
		}
	}
	if !helper {
		multi := filepath.Join(root, "MultiServer")
		c := exec.Command(filepath.Join(multi, "NetherNetMultiServer.exe"), filepath.Join(multi, "config.json"))
		c.Dir = multi
		c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		out, e := os.OpenFile(filepath.Join(multi, "helper-runtime.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		defer out.Close()
		errlog, e := os.OpenFile(filepath.Join(multi, "helper-errors.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		defer errlog.Close()
		c.Stdout = out
		c.Stderr = errlog
		if e = c.Start(); e != nil {
			return e
		}
		go c.Wait()
	}
	for i := 0; i < 30; i++ {
		var statuses []supervisor.Status
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		e = adminCall(callCtx, "servers.list", &statuses)
		cancel()
		if e == nil {
			for _, s := range statuses {
				wanted := ids == nil
				for _, id := range ids {
					if s.ID == id {
						wanted = true
					}
				}
				if wanted && (s.State == supervisor.Stopped || s.State == supervisor.Failed) {
					if e = adminCallParams(ctx, "server.start", map[string]string{"server_id": s.ID}); e != nil {
						return e
					}
				}
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("helper administration did not become ready")
}
func adminCallParams(ctx context.Context, method string, params any) error {
	// Use the same framed protocol for starting individual managed servers.
	return callAdminParameters(ctx, method, params)
}
func RuntimeReport(ctx context.Context, root string) ([]RuntimeCheck, error) {
	exe, e := os.Executable()
	if e != nil {
		return nil, e
	}
	script, e := VerifiedScript(exe, "Runtime-Operations.ps1")
	if e != nil {
		return nil, e
	}
	c := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Operation", "Report", "-Root", root)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	b, e := c.CombinedOutput()
	if e != nil {
		return nil, fmt.Errorf("endpoint checks blocked: %s", b)
	}
	var report []RuntimeCheck
	e = json.Unmarshal(b, &report)
	return report, e
}
