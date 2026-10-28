//go:build windows

package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"net"
	"nethernet-multi-server/internal/adminapi"
	"nethernet-multi-server/internal/adminpipe"
	"nethernet-multi-server/internal/config"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type PipeClient struct{}

func (PipeClient) Call(ctx context.Context, m string, p, t any) error {
	conn, err := adminpipe.DialCurrentUser(ctx, adminpipe.DefaultName)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err = adminapi.WriteRequest(conn, adminapi.Request{ID: "fleet", Method: m, Params: b}); err != nil {
		return err
	}
	r, err := adminapi.ReadResponse(conn)
	if err != nil {
		return err
	}
	if r.Error != nil {
		return errors.New(r.Error.Message)
	}
	if t != nil {
		b, err = json.Marshal(r.Result)
		if err == nil {
			err = json.Unmarshal(b, t)
		}
	}
	return err
}

type NativeHost struct {
	Directory string
	Client    Client
	mu        sync.Mutex
}

func (h *NativeHost) helperPIDs() ([]uint32, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	var found []uint32
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "NetherNetMultiServer.exe") {
			continue
		}
		process, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if e != nil {
			return nil, fmt.Errorf("cannot verify helper ownership: %w", e)
		}
		buf := make([]uint16, 32768)
		size := uint32(len(buf))
		e = windows.QueryFullProcessImageName(process, 0, &buf[0], &size)
		windows.CloseHandle(process)
		if e != nil {
			return nil, e
		}
		if !strings.EqualFold(windows.UTF16ToString(buf[:size]), filepath.Join(h.Directory, "NetherNetMultiServer.exe")) {
			return nil, errors.New("another Minecraft installation's helper is running; refusing to take it over")
		}
		found = append(found, entry.ProcessID)
	}
	return found, nil
}
func (h *NativeHost) Running() (bool, error) { ids, err := h.helperPIDs(); return len(ids) > 0, err }

// A missing helper does not imply its former child servers have exited.
func (h *NativeHost) VerifyStopped() error {
	cfg, err := config.Load(filepath.Join(h.Directory, "config.json"))
	if err != nil {
		return err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		for _, server := range cfg.Servers {
			if !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), filepath.Base(server.Process.Executable)) {
				continue
			}
			process, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
			if e != nil {
				return fmt.Errorf("cannot verify server ownership: %w", e)
			}
			buf := make([]uint16, 32768)
			size := uint32(len(buf))
			e = windows.QueryFullProcessImageName(process, 0, &buf[0], &size)
			windows.CloseHandle(process)
			if e != nil {
				return e
			}
			if strings.EqualFold(filepath.Clean(windows.UTF16ToString(buf[:size])), filepath.Clean(server.Process.Executable)) {
				return fmt.Errorf("%s is still running without its helper; stop it gracefully from its original console before continuing", server.Name)
			}
		}
	}
	return nil
}
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (h *NativeHost) Ensure(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	mutex, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(`Local\NetherNet.HelperLaunch`))
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		return err
	}
	defer windows.CloseHandle(mutex)
	for {
		r, e := windows.WaitForSingleObject(mutex, 100)
		if e != nil {
			return e
		}
		if r == windows.WAIT_OBJECT_0 || r == windows.WAIT_ABANDONED {
			break
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	defer windows.ReleaseMutex(mutex)
	for _, marker := range []string{"installation-retired", "network-maintenance", "migration-activation-required"} {
		if _, e := os.Stat(filepath.Join(filepath.Dir(h.Directory), marker)); e == nil {
			return fmt.Errorf("startup paused by %s; finish migration/network maintenance first", marker)
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	cfg, err := config.Load(filepath.Join(h.Directory, "config.json"))
	if err != nil {
		return err
	}
	networkDeadline := time.Now().Add(3 * time.Minute)
	for {
		local := map[string]bool{}
		addrs, e := net.InterfaceAddrs()
		if e != nil {
			return e
		}
		for _, a := range addrs {
			ip, _, _ := net.ParseCIDR(a.String())
			local[ip.String()] = true
		}
		required := []string{cfg.InterfaceIP}
		for _, s := range cfg.Servers {
			ip, _, e := net.SplitHostPort(s.Backend)
			if e != nil {
				return e
			}
			required = append(required, ip)
		}
		missing := ""
		for _, ip := range required {
			if !local[ip] {
				missing = ip
				break
			}
		}
		if missing == "" {
			break
		}
		if time.Now().After(networkDeadline) {
			return fmt.Errorf("configured IP %s is unavailable; use Repair Network Configuration", missing)
		}
		if err := sleepContext(ctx, time.Second); err != nil {
			return err
		}
	}
	running, err := h.Running()
	if err != nil {
		return err
	}
	if !running {
		file, e := os.OpenFile(filepath.Join(h.Directory, "panel-runtime.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if e != nil {
			return e
		}
		cmd := exec.Command(filepath.Join(h.Directory, "NetherNetMultiServer.exe"), filepath.Join(h.Directory, "config.json"))
		cmd.Dir = h.Directory
		cmd.Stdout = file
		cmd.Stderr = file
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
		if e = cmd.Start(); e != nil {
			file.Close()
			return e
		}
		go func() { _ = cmd.Wait(); file.Close() }()
	}
	deadline := time.Now().Add(time.Minute)
	for {
		query, cancel := context.WithTimeout(ctx, 2*time.Second)
		var states []supervisor.Status
		err = h.Client.Call(query, "servers.list", struct{}{}, &states)
		cancel()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("helper did not become ready: %w", err)
		}
		if e := sleepContext(ctx, 300*time.Millisecond); e != nil {
			return e
		}
	}
}
func (h *NativeHost) WaitStopped(ctx context.Context) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		running, err := h.Running()
		if err != nil {
			return err
		}
		if !running {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("helper has not exited cleanly; no process was force-stopped")
		}
		if err := sleepContext(ctx, 200*time.Millisecond); err != nil {
			return err
		}
	}
}
func ReadyProbe(client Client) func(context.Context, supervisor.Status) error {
	return func(ctx context.Context, initial supervisor.Status) error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		var stable time.Time
		pid := 0
		for {
			var p supervisor.Progress
			query, done := context.WithTimeout(ctx, 3*time.Second)
			err := client.Call(query, "server.progress", map[string]string{"server_id": initial.ID}, &p)
			done()
			if err == nil {
				if p.Status.State == supervisor.Failed || p.Status.State == supervisor.Stopped {
					return fmt.Errorf("Minecraft stopped during startup: %s", p.Status.LastError)
				}
				if pid != 0 && p.Status.PID != pid {
					return errors.New("server process changed during readiness verification")
				}
				pid = p.Status.PID
				ready := p.Status.State == supervisor.Running && (p.Status.Ready || strings.Contains(p.Log, "Server started."))
				if ready {
					probe, finish := context.WithTimeout(ctx, 750*time.Millisecond)
					conn, e := (&net.Dialer{}).DialContext(probe, "tcp4", p.Status.Backend)
					finish()
					if e == nil {
						conn.Close()
					} else {
						ready = false
					}
				}
				if ready {
					if stable.IsZero() {
						stable = time.Now()
					}
					if time.Since(stable) >= 10*time.Second {
						return nil
					}
				} else {
					stable = time.Time{}
				}
			}
			if err != nil {
				stable = time.Time{}
			}
			if err := sleepContext(ctx, time.Second); err != nil {
				return fmt.Errorf("startup verification timed out: %w", err)
			}
		}
	}
}
func NativeEngine(directory string) *Engine {
	client := PipeClient{}
	return &Engine{Client: client, Host: &NativeHost{Directory: directory, Client: client}, Ready: ReadyProbe(client)}
}
