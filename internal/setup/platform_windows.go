package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const ConfigurationScriptSHA256 = "773fc14ee1b5e76917ffe9f9bd58901cb128be064b8853f8a84d1fb54bcaf8c7"

func VerifiedScript(executable, name string) (string, error) {
	expected, ok := ScriptHashes[name]
	if !ok {
		return "", errors.New("unrecognised setup script")
	}
	path := filepath.Join(filepath.Dir(executable), "setup-assets", name)
	b, e := os.ReadFile(path)
	if e != nil {
		return "", errors.New("keep the complete installer and setup-assets folder together")
	}
	h := sha256.Sum256(b)
	if !strings.EqualFold(hex.EncodeToString(h[:]), expected) {
		return "", errors.New("Windows setup script checksum failed: " + name)
	}
	return path, nil
}
func configurationScript(executable string) (string, error) {
	for name := range ScriptHashes {
		if _, e := VerifiedScript(executable, name); e != nil {
			return "", e
		}
	}
	return VerifiedScript(executable, "Configure-Windows.ps1")
}

type shellExecuteInfo struct {
	Size, Mask                        uint32
	Window                            windows.Handle
	Verb, File, Parameters, Directory *uint16
	Show                              int32
	Instance                          windows.Handle
	IDList                            uintptr
	Class                             *uint16
	ClassKey                          windows.Handle
	HotKey                            uint32
	Icon, Process                     windows.Handle
}

var shellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// Use native Windows UAC and a readable, checksum-verified configuration script.
func ElevateApply(ctx context.Context, p Plan) error {
	executable, e := os.Executable()
	if e != nil {
		return e
	}
	script, e := configurationScript(executable)
	if e != nil {
		return e
	}
	file, e := os.CreateTemp(filepath.Join(p.Root, "MultiServer"), ".setup-plan-*.json")
	if e != nil {
		return e
	}
	planPath := file.Name()
	defer os.Remove(planPath)
	e = json.NewEncoder(file).Encode(p)
	closeError := file.Close()
	if e != nil {
		return e
	}
	if closeError != nil {
		return closeError
	}
	_ = os.Remove(filepath.Join(p.Root, "setup-result.json"))
	verb, _ := windows.UTF16PtrFromString("runas")
	program, _ := windows.UTF16PtrFromString(filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"))
	arguments := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-PlanPath", planPath}
	for i, a := range arguments {
		arguments[i] = syscall.EscapeArg(a)
	}
	parameters, e := windows.UTF16PtrFromString(strings.Join(arguments, " "))
	if e != nil {
		return e
	}
	info := shellExecuteInfo{Mask: 0x40, Verb: verb, File: program, Parameters: parameters, Show: 0}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, callError := shellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return fmt.Errorf("Windows administrator approval was cancelled or failed: %v", callError)
	}
	defer windows.CloseHandle(info.Process)
	for {
		result, e := windows.WaitForSingleObject(info.Process, 250)
		if e != nil {
			return e
		}
		if result == windows.WAIT_OBJECT_0 {
			break
		}
		if e = ctx.Err(); e != nil {
			return e
		}
	}
	var code uint32
	if e = windows.GetExitCodeProcess(info.Process, &code); e != nil {
		return e
	}
	if code == 0 {
		return nil
	}
	if b, e := os.ReadFile(filepath.Join(p.Root, "setup-result.json")); e == nil {
		var result struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(b, &result) == nil && result.Error != "" {
			return errors.New(result.Error)
		}
	}
	return errors.New("Windows configuration did not complete; correct the issue and retry Configure and activate")
}
