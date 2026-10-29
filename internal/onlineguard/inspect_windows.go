//go:build windows

package onlineguard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// Inspect reads the effective ActiveStore and complete host IP inventory. It
// invokes only Get-* cmdlets and never elevates or changes machine state.
func Inspect(ctx context.Context) (Evidence, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	root := os.Getenv("SystemRoot")
	if root == "" {
		return Evidence{}, errors.New("Windows system directory unavailable")
	}
	cmd := exec.CommandContext(ctx, filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", inspectionScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	b, e := cmd.Output()
	if e != nil {
		// On a standard Windows install, ActiveStore inspection can be restricted
		// to an elevated token. A dedicated SYSTEM task may write the exact same
		// read-only snapshot to ProgramData. The evaluator still rejects a stale,
		// incomplete, or mismatching snapshot, so this is never an availability
		// bypass for the Online gateway.
		if cached, cacheErr := cachedEvidence(); cacheErr == nil {
			return cached, nil
		}
		return Evidence{}, errors.New("read-only firewall inspection unavailable; no protection verified")
	}
	if len(b) > 16<<20 {
		return Evidence{}, errors.New("firewall evidence exceeds size limit")
	}
	var evidence Evidence
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(&evidence); e != nil {
		return Evidence{}, errors.New("invalid firewall inspection evidence")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return Evidence{}, errors.New("trailing firewall inspection output")
	}
	return evidence, nil
}

func cachedEvidence() (Evidence, error) {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	b, err := os.ReadFile(filepath.Join(programData, "NetherNet", "onlineguard-evidence.json"))
	if err != nil || len(b) == 0 || len(b) > 16<<20 {
		return Evidence{}, errors.New("cached firewall evidence unavailable")
	}
	var evidence Evidence
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(&evidence); err != nil {
		return Evidence{}, errors.New("cached firewall evidence is invalid")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return Evidence{}, errors.New("cached firewall evidence has trailing output")
	}
	return evidence, nil
}

const inspectionScript = `
$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
function Values($value){ @($value | ForEach-Object { [string]$_ } | Where-Object { $_ -ne '' }) }
function AnyValue($value){ if(-not $value){return 'Any'};[string]$value }
$before=@(Get-NetIPAddress | ForEach-Object { (([string]$_.IPAddress -split '%')[0]) } | Sort-Object -Unique)
$profiles=@(Get-NetFirewallProfile -PolicyStore ActiveStore | ForEach-Object {
 [pscustomobject]@{Name=[string]$_.Name;Enabled=([string]$_.Enabled -eq 'True');DefaultInboundAction=[string]$_.DefaultInboundAction;AllowInboundRules=([string]$_.AllowInboundRules -eq 'True');DisabledInterfaceAliases=@(Values $_.DisabledInterfaceAliases)}
})
$rules=@(Get-NetFirewallRule -PolicyStore ActiveStore -Direction Inbound -Enabled True | ForEach-Object {
 $r=$_
 $a=$r|Get-NetFirewallApplicationFilter
 $p=$r|Get-NetFirewallPortFilter
 $ad=$r|Get-NetFirewallAddressFilter
 $s=$r|Get-NetFirewallServiceFilter
 $i=$r|Get-NetFirewallInterfaceFilter
 $it=$r|Get-NetFirewallInterfaceTypeFilter
 $security=$r|Get-NetFirewallSecurityFilter
 $profileNames=@(([string]$r.Profile -split ',\s*'))
 if([string]$r.Profile -eq 'Any'){$profileNames=@('Domain','Private','Public')}
 [pscustomobject]@{PrimaryStatus=[string]$r.PrimaryStatus;EnforcementStatus=(@($r.EnforcementStatus|ForEach-Object{[string]$_}) -join ',');Spec=[pscustomobject]@{
 DynamicTarget=[string]$p.DynamicTarget;ICMPTypes=@(Values $p.IcmpType);Owner=[string]$r.Owner;PolicyAppID=[string]$a.PolicyAppId;LooseSourceMapping=[bool]$r.LooseSourceMapping;LocalOnlyMapping=[bool]$r.LocalOnlyMapping;RemoteDynamicKeywordAddresses=@(Values $ad.RemoteDynamicKeywordAddresses)
 Name=[string]$r.Name;Group=[string]$r.Group;Program=[string]$a.Program;Direction=[string]$r.Direction;Action=[string]$r.Action;Enabled=([string]$r.Enabled -eq 'True');Profiles=$profileNames
 Protocol=[string]$p.Protocol;LocalAddresses=@(Values $ad.LocalAddress);RemoteAddresses=@(Values $ad.RemoteAddress);LocalPorts=@(Values $p.LocalPort);RemotePorts=@(Values $p.RemotePort)
 InterfaceAliases=@(Values $i.InterfaceAlias);InterfaceTypes=@(Values $it.InterfaceType);Service=[string]$s.Service;Package=(AnyValue $a.Package)
 Authentication=[string]$security.Authentication;Encryption=[string]$security.Encryption;OverrideBlockRules=([string]$security.OverrideBlockRules -eq 'True')
 EdgeTraversalPolicy=[string]$r.EdgeTraversalPolicy;LocalUsers=@(Values $security.LocalUser);RemoteUsers=@(Values $security.RemoteUser);RemoteMachines=@(Values $security.RemoteMachine);Platforms=@(Values $r.Platforms)
 }}
})
$after=@(Get-NetIPAddress | ForEach-Object { (([string]$_.IPAddress -split '%')[0]) } | Sort-Object -Unique)
if(($before -join ',') -ne ($after -join ',')){throw 'Host addresses changed during inspection'}
[pscustomobject]@{Platform='windows';Complete=$true;ObservedAt=[DateTime]::UtcNow.ToString('o');FirewallServiceRunning=((Get-Service MpsSvc).Status -eq 'Running');BaseFilteringEngineRunning=((Get-Service BFE).Status -eq 'Running');LocalAddresses=$after;Profiles=$profiles;Rules=$rules}|ConvertTo-Json -Depth 12 -Compress
`
