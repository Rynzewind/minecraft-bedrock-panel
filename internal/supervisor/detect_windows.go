package supervisor

import (
	"net"
	"nethernet-multi-server/internal/config"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func parseNetstatUDPPID(output, backend string) int {
	_, port, _ := net.SplitHostPort(backend)
	for _, line := range strings.Split(output, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || !strings.EqualFold(f[0], "UDP") {
			continue
		}
		local := f[1]
		if local != backend && !strings.HasSuffix(local, ":"+port) {
			continue
		}
		pid, err := strconv.Atoi(f[len(f)-1])
		if err == nil {
			return pid
		}
	}
	return 0
}
func (m *Manager) DetectExternal(servers []config.Server) {
	processes, processErr := runningProcessPaths()
	b, netstatErr := exec.Command("netstat.exe", "-ano", "-p", "udp").Output()
	for _, s := range servers {
		pid := 0
		if processErr == nil && s.Process != nil {
			pid = configuredProcessPID(processes, s.Process.Executable)
		}
		if pid == 0 && netstatErr == nil {
			pid = parseNetstatUDPPID(string(b), s.Backend)
		}
		_ = m.observeExternal(s.ID, pid)
	}
}

func configuredProcessPID(processes map[string]int, executable string) int {
	return processes[strings.ToLower(filepath.Clean(executable))]
}

func runningProcessPaths() (map[string]int, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)
	result := map[string]int{}
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err = windows.Process32First(snapshot, &entry); err != nil {
		return nil, err
	}
	for {
		handle, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if openErr == nil {
			buffer := make([]uint16, 32768)
			size := uint32(len(buffer))
			if queryErr := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); queryErr == nil {
				path := strings.ToLower(filepath.Clean(windows.UTF16ToString(buffer[:size])))
				result[path] = int(entry.ProcessID)
			}
			windows.CloseHandle(handle)
		}
		if err = windows.Process32Next(snapshot, &entry); err != nil {
			if err == windows.ERROR_NO_MORE_FILES {
				return result, nil
			}
			return nil, err
		}
	}
}
