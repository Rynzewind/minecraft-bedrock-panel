package migration

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"nethernet-multi-server/internal/accountstore"
	"nethernet-multi-server/internal/config"
	"os"
	"path/filepath"
	"strings"
)

type RestoreOptions struct {
	ArchivePath, Destination, LANAddress, BroadcastAddress string
	OldDeploymentStopped                                   bool
	ServerAddresses                                        map[string]string
}
type RestoreResult struct {
	Root                    string
	AccountsRequiringSignIn int
	ActivationRequired      bool
}

func Restore(ctx context.Context, opts RestoreOptions) (result RestoreResult, err error) {
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if !filepath.IsAbs(opts.Destination) {
		return result, errors.New("destination must be absolute")
	}
	dst := filepath.Clean(opts.Destination)
	if strings.ContainsAny(dst, "%\r\n\"!`$") {
		return result, errors.New("destination cannot contain shell expansion characters")
	}
	if err = noLinks(dst); err != nil {
		return result, err
	}
	if _, e := os.Lstat(dst); !os.IsNotExist(e) {
		return result, errors.New("choose a new destination folder that does not already exist")
	}
	ip, broadcast := net.ParseIP(opts.LANAddress), net.ParseIP(opts.BroadcastAddress)
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() || broadcast == nil || broadcast.To4() == nil || broadcast.IsUnspecified() || broadcast.IsLoopback() || broadcast.IsMulticast() {
		return result, errors.New("select a valid LAN IPv4 interface and subnet broadcast")
	}
	z, err := zip.OpenReader(opts.ArchivePath)
	if err != nil {
		return result, err
	}
	defer z.Close()
	m, err := verifyZip(ctx, &z.Reader)
	if err != nil {
		return result, err
	}
	var required uint64
	for _, f := range m.Files {
		required += uint64(f.Size)
	}
	parent := filepath.Dir(dst)
	if err = os.MkdirAll(parent, 0700); err != nil {
		return result, err
	}
	if err = checkSpace(parent, required+(256<<20)); err != nil {
		return result, err
	}
	stage, err := os.MkdirTemp(parent, ".minecraft-restore-")
	if err != nil {
		return result, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	if err = privateDirectory(stage); err != nil {
		return result, err
	}
	for _, f := range z.File {
		if f.Name == "manifest.json" {
			continue
		}
		if err = ctx.Err(); err != nil {
			return result, err
		}
		p := filepath.Join(stage, filepath.FromSlash(f.Name))
		if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			return result, err
		}
		r, e := f.Open()
		if e != nil {
			return result, e
		}
		w, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			r.Close()
			return result, e
		}
		_, e = io.Copy(w, contextReader{ctx, r})
		r.Close()
		closeErr := w.Close()
		if e != nil {
			return result, e
		}
		if closeErr != nil {
			return result, closeErr
		}
	}
	for _, f := range m.Files {
		entry, e := fileEntry(stage, filepath.Join(stage, filepath.FromSlash(f.Path)))
		if e != nil {
			return result, e
		}
		if entry.Size != f.Size || !strings.EqualFold(entry.SHA256, f.SHA256) {
			return result, errors.New("payload changed during extraction")
		}
	}
	sourceManifest := filepath.Join(stage, "MultiServer", "installation-resources.json")
	if _, e := os.Stat(sourceManifest); e == nil {
		if e = os.Rename(sourceManifest, filepath.Join(stage, "MultiServer", "source-installation-resources.json")); e != nil {
			return result, e
		}
	}
	configPath := filepath.Join(stage, "MultiServer", "config.json")
	cfg, err := config.Load(configPath)
	if err != nil {
		return result, err
	}
	rebase := func(p string) (string, error) {
		if !filepath.IsAbs(p) {
			if !filepath.IsLocal(p) {
				return "", errors.New("relative content path escapes the panel directory")
			}
			p = filepath.Join(m.SourceRoot, "MultiServer", p)
		}
		if !inside(m.SourceRoot, p) {
			return "", errors.New("configuration path is outside snapshot root")
		}
		r, e := filepath.Rel(m.SourceRoot, p)
		if e != nil {
			return "", e
		}
		return filepath.Join(dst, r), nil
	}
	cfg.InterfaceIP = opts.LANAddress
	_, port, e := net.SplitHostPort(cfg.Listen)
	if e != nil {
		return result, e
	}
	cfg.Listen = net.JoinHostPort(opts.BroadcastAddress, port)
	for i := range cfg.Servers {
		s := &cfg.Servers[i]
		if _, e := os.Stat(filepath.Join(stage, "MultiServer", "identities", s.NetworkID+".pem")); e != nil {
			return result, errors.New("snapshot is missing a persistent server identity")
		}
		if s.Process == nil {
			return result, errors.New("unmanaged server in migration")
		}
		_, port, e = net.SplitHostPort(s.Backend)
		if e != nil {
			return result, e
		}
		address := opts.LANAddress
		if opts.ServerAddresses != nil {
			var ok bool
			address, ok = opts.ServerAddresses[s.ID]
			if !ok || net.ParseIP(address).To4() == nil {
				return result, errors.New("enter a valid IPv4 address for every server")
			}
		}
		s.Backend = net.JoinHostPort(address, port)
		for _, field := range []*string{&s.Process.Executable, &s.Process.WorkingDirectory, &s.Process.WorldDirectory, &s.Process.LogDirectory} {
			*field, err = rebase(*field)
			if err != nil {
				return result, err
			}
		}
		workRel, _ := filepath.Rel(dst, s.Process.WorkingDirectory)
		propertiesPath := filepath.Join(stage, workRel, "server.properties")
		if err = SynchroniseServerIP(propertiesPath, address); err != nil {
			return result, err
		}
		exeRel, _ := filepath.Rel(dst, s.Process.Executable)
		if _, err = os.Stat(filepath.Join(stage, exeRel)); err != nil {
			return result, errors.New("snapshot is missing a configured server executable")
		}
		worldRel, _ := filepath.Rel(dst, s.Process.WorldDirectory)
		if i, e := os.Stat(filepath.Join(stage, worldRel)); e != nil || !i.IsDir() {
			return result, errors.New("snapshot is missing a configured world")
		}
	}
	if err = cfg.Validate(); err != nil {
		return result, err
	}
	if err = writeJSON(configPath, cfg); err != nil {
		return result, err
	}
	if err = rebasePackState(filepath.Join(stage, "MultiServer", "content-staging", "state.json"), rebase); err != nil {
		return result, err
	}
	metadata, err := os.ReadFile(filepath.Join(stage, "accounts-migration.json"))
	if err != nil {
		return result, err
	}
	accounts, err := ImportAccounts(metadata)
	if err != nil {
		return result, err
	}
	store := accountstore.New(filepath.Join(stage, "MultiServer", "account-store", "accounts.json"), accountstore.DPAPIProtector{Entropy: []byte("NetherNetMultiServer/account-store/v1")})
	if err = store.Save(accounts); err != nil {
		return result, err
	}
	if err = writeLaunchers(stage); err != nil {
		return result, err
	}
	if err = os.WriteFile(filepath.Join(stage, "migration-activation-required"), []byte("Complete setup and confirm the old deployment is stopped before enabling LAN advertising.\n"), 0600); err != nil {
		return result, err
	}
	for _, name := range []string{"NetherNetMultiServer.exe", "NetherNetMultiServerAdmin.exe", "NetherNetMultiServerControl.exe"} {
		if _, err = os.Stat(filepath.Join(stage, "MultiServer", name)); err != nil {
			return result, errors.New("snapshot is missing required panel binaries")
		}
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if _, e = os.Lstat(dst); !os.IsNotExist(e) {
		return result, errors.New("destination appeared during restore; refusing overwrite")
	}
	if err = os.Rename(stage, dst); err != nil {
		return result, err
	}
	return RestoreResult{Root: dst, AccountsRequiringSignIn: len(accounts.Accounts), ActivationRequired: true}, nil
}
func writeJSON(p string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(p, b, 0600)
}
func rebasePackState(p string, rebase func(string) (string, error)) error {
	b, e := os.ReadFile(p)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	var state map[string]json.RawMessage
	if e = json.Unmarshal(b, &state); e != nil {
		return e
	}
	var packs []map[string]json.RawMessage
	if e = json.Unmarshal(state["packs"], &packs); e != nil {
		return e
	}
	for _, pack := range packs {
		if raw, ok := pack["installed_path"]; ok {
			var old string
			if e = json.Unmarshal(raw, &old); e != nil {
				return e
			}
			next, e := rebase(old)
			if e != nil {
				return e
			}
			pack["installed_path"], _ = json.Marshal(next)
		}
	}
	state["packs"], _ = json.Marshal(packs)
	return writeJSON(p, state)
}
func writeLaunchers(root string) error {
	start := `$ErrorActionPreference='Stop'
$multi=Join-Path $PSScriptRoot 'MultiServer'
$helper=Join-Path $multi 'NetherNetMultiServer.exe'
$control=Join-Path $multi 'NetherNetMultiServerControl.exe'
if (-not (Get-Process NetherNetMultiServer -ErrorAction SilentlyContinue)) {
 Start-Process -FilePath $helper -WorkingDirectory $multi -WindowStyle Hidden -RedirectStandardOutput (Join-Path $multi 'helper-runtime.log') -RedirectStandardError (Join-Path $multi 'helper-errors.log')
}
for ($attempt=0;$attempt -lt 30;$attempt++) {
 & $control start-all
 if ($LASTEXITCODE -eq 0) {exit 0}
 Start-Sleep -Seconds 1
}
throw 'Helper did not become ready. Check MultiServer/helper-errors.log.'
`
	files := map[string]string{
		"StartMinecraftServers.ps1":                  start,
		"StartMinecraftServers.bat":                  "@echo off\r\npowershell.exe -NoProfile -ExecutionPolicy Bypass -File \"%~dp0StartMinecraftServers.ps1\"\r\n",
		"StopMinecraftServers.bat":                   "@echo off\r\n\"%~dp0MultiServer\\NetherNetMultiServerControl.exe\" stop-all\r\n",
		"MultiServer/Run-NetherNetMultiServer.cmd":   "@echo off\r\ncd /d \"%~dp0\"\r\n\"%~dp0NetherNetMultiServer.exe\" \"%~dp0config.json\"\r\n",
		"MultiServer/Start-NetherNetMultiServer.cmd": "@echo off\r\ncall \"%~dp0..\\StartMinecraftServers.bat\"\r\n",
		"StartMinecraftServers.vbs":                  "Set shell = CreateObject(\"WScript.Shell\")\r\nSet fso = CreateObject(\"Scripting.FileSystemObject\")\r\nroot = fso.GetParentFolderName(WScript.ScriptFullName)\r\nshell.Run Chr(34) & root & \"\\StartMinecraftServers.bat\" & Chr(34), 0, False\r\n",
	}
	for name, text := range files {
		if e := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(text), 0600); e != nil {
			return e
		}
	}
	return nil
}
