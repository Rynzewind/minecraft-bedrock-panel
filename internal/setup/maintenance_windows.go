package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"nethernet-multi-server/internal/adminapi"
	"nethernet-multi-server/internal/adminpipe"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"
)

type InstalledProcess struct {
	PID  uint32
	Path string
}

func InstallationProcesses(root string) ([]InstalledProcess, error) {
	snapshot, e := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if e != nil {
		return nil, e
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	var out []InstalledProcess
	for e = windows.Process32First(snapshot, &entry); e == nil; e = windows.Process32Next(snapshot, &entry) {
		name := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		if name != "bedrock_server.exe" && name != "nethernetmultiserver.exe" {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if err != nil {
			return nil, fmt.Errorf("cannot verify process %s: %w", name, err)
		}
		buf := make([]uint16, 32768)
		size := uint32(len(buf))
		err = windows.QueryFullProcessImageName(h, 0, &buf[0], &size)
		windows.CloseHandle(h)
		if err != nil {
			return nil, err
		}
		path := windows.UTF16ToString(buf[:size])
		if strings.HasPrefix(strings.ToLower(path), strings.ToLower(filepath.Clean(root))+`\`) {
			out = append(out, InstalledProcess{entry.ProcessID, path})
		}
	}
	return out, nil
}
func adminCall(ctx context.Context, method string, target any) error {
	conn, e := adminpipe.DialCurrentUser(ctx, adminpipe.DefaultName)
	if e != nil {
		return e
	}
	defer conn.Close()
	if d, ok := ctx.Deadline(); ok {
		conn.SetDeadline(d)
	}
	if e = adminapi.WriteRequest(conn, adminapi.Request{ID: "network-setup", Method: method, Params: json.RawMessage(`{}`)}); e != nil {
		return e
	}
	r, e := adminapi.ReadResponse(conn)
	if e != nil {
		return e
	}
	if r.Error != nil {
		return fmt.Errorf("%s", r.Error.Message)
	}
	if target != nil {
		b, _ := json.Marshal(r.Result)
		return json.Unmarshal(b, target)
	}
	return nil
}
func PrepareForNetworkChange(ctx context.Context, root string) ([]string, error) {
	processes, e := InstallationProcesses(root)
	if e != nil {
		return nil, e
	}
	helper := false
	for _, p := range processes {
		if strings.EqualFold(p.Path, filepath.Join(root, "MultiServer", "NetherNetMultiServer.exe")) {
			helper = true
		}
	}
	if len(processes) == 0 {
		return nil, nil
	}
	if !helper {
		return nil, fmt.Errorf("stop the installation's externally running servers cleanly before network configuration")
	}
	var statuses []supervisor.Status
	if e = adminCall(ctx, "servers.list", &statuses); e != nil {
		return nil, e
	}
	var running []string
	for _, s := range statuses {
		if s.State == supervisor.Running {
			running = append(running, s.ID)
		}
		if s.State != supervisor.Running && s.State != supervisor.Stopped && s.State != supervisor.Failed {
			return nil, fmt.Errorf("%s is not in a safe maintenance state", s.ID)
		}
	}
	marker := filepath.Join(root, "network-maintenance")
	if e = os.WriteFile(marker, []byte("Network repair in progress; startup is paused."), 0600); e != nil {
		return nil, e
	}
	if e = adminCall(ctx, "system.shutdown", nil); e != nil {
		os.Remove(marker)
		return nil, e
	}
	for i := 0; i < 50; i++ {
		processes, e = InstallationProcesses(root)
		if e != nil {
			return nil, e
		}
		if len(processes) == 0 {
			return running, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, fmt.Errorf("processes have not stopped; network configuration was not applied")
}
func callAdminParameters(ctx context.Context, method string, params any) error {
	conn, e := adminpipe.DialCurrentUser(ctx, adminpipe.DefaultName)
	if e != nil {
		return e
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	b, e := json.Marshal(params)
	if e != nil {
		return e
	}
	if e = adminapi.WriteRequest(conn, adminapi.Request{ID: "network-start", Method: method, Params: b}); e != nil {
		return e
	}
	r, e := adminapi.ReadResponse(conn)
	if e != nil {
		return e
	}
	if r.Error != nil {
		return fmt.Errorf("%s", r.Error.Message)
	}
	return nil
}
