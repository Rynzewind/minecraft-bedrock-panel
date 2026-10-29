//go:build windows

package onlineguard

import (
	"errors"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Prove the sole LocalUser SID is absent from every configured backend token.
// Missing processes, unreadable/restricted tokens and unknown SDDL fail closed.
func unrelatedLocalUserBlock(rule RuleSpec, backends []Backend) bool {
	sidText := localUserSID(rule.LocalUsers)
	if sidText == "" || len(backends) == 0 {
		return false
	}
	sid, err := windows.StringToSid(sidText)
	if err != nil {
		return false
	}
	wanted := map[string]bool{}
	for _, b := range backends {
		wanted[strings.ToLower(b.Program)] = false
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "bedrock_server.exe") {
			if !checkBackendProcess(entry.ProcessID, sid, wanted) {
				return false
			}
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return false
	}
	for _, seen := range wanted {
		if !seen {
			return false
		}
	}
	return true
}

func checkBackendProcess(pid uint32, sid *windows.SID, wanted map[string]bool) bool {
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(p)
	name := make([]uint16, 32768)
	size := uint32(len(name))
	if windows.QueryFullProcessImageName(p, 0, &name[0], &size) != nil {
		return false
	}
	program := strings.ToLower(windows.UTF16ToString(name[:size]))
	if _, configured := wanted[program]; !configured {
		return true
	}
	var token windows.Token
	if windows.OpenProcessToken(p, windows.TOKEN_QUERY, &token) != nil {
		return false
	}
	defer token.Close()
	restricted, err := token.IsRestricted()
	if err != nil || restricted {
		return false
	}
	user, err := token.GetTokenUser()
	if err != nil || windows.EqualSid(user.User.Sid, sid) {
		return false
	}
	groups, err := token.GetTokenGroups()
	if err != nil {
		return false
	}
	// Include disabled and deny-only groups as a conservative superset.
	for _, group := range groups.AllGroups() {
		if windows.EqualSid(group.Sid, sid) {
			return false
		}
	}
	wanted[program] = true
	return true
}
