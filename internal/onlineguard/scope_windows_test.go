//go:build windows

package onlineguard

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestBackendTokenScopeUsesActualUserAndGroups(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	program := strings.ToLower(exe)
	wanted := map[string]bool{program: false}
	if checkBackendProcess(uint32(os.Getpid()), user.User.Sid, wanted) {
		t.Fatal("block on actual user ignored")
	}
	groups, err := windows.GetCurrentProcessToken().GetTokenGroups()
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range groups.AllGroups() {
		if checkBackendProcess(uint32(os.Getpid()), group.Sid, map[string]bool{program: false}) {
			t.Fatal("block on token group ignored")
		}
	}
	absent, err := windows.StringToSid("S-1-5-21-123-456-789-1009")
	if err != nil {
		t.Fatal(err)
	}
	restricted, err := windows.GetCurrentProcessToken().IsRestricted()
	if err != nil {
		t.Fatal(err)
	}
	if restricted {
		if checkBackendProcess(uint32(os.Getpid()), absent, wanted) {
			t.Fatal("restricted token accepted as proof")
		}
		return
	}
	if !checkBackendProcess(uint32(os.Getpid()), absent, wanted) || !wanted[program] {
		t.Fatal("unrelated user scope not proven")
	}
	if checkBackendProcess(0, absent, wanted) {
		t.Fatal("unreadable process accepted")
	}
}

func TestAbsentBackendDoesNotProveDisjointScope(t *testing.T) {
	rule := RuleSpec{LocalUsers: []string{"D:(A;;CC;;;S-1-5-21-123-456-789-1009)"}}
	if unrelatedLocalUserBlock(rule, []Backend{{Program: `C:\missing\bedrock_server.exe`}}) {
		t.Fatal("absent backend scope accepted")
	}
}
