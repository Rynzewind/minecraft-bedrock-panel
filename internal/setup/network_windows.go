package setup

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

func DetectInterfaces() ([]LANInterface, error) {
	exe, e := os.Executable()
	if e != nil {
		return nil, e
	}
	script, e := VerifiedScript(exe, "Network-Configuration.ps1")
	if e != nil {
		return nil, e
	}
	c := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Operation", "Detect")
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	b, e := c.Output()
	if e != nil {
		return nil, errors.New("Windows network detection failed; run setup with permission to read network adapters")
	}
	var rows []struct {
		AdapterGUID string `json:"adapter_guid"`
		Index       int    `json:"interface_index"`
		Name        string `json:"name"`
		IP          string `json:"primary_ip"`
		Prefix      int    `json:"prefix_length"`
		Gateway     string `json:"gateway"`
		DHCP        string `json:"dhcp"`
		Secondary   bool   `json:"skip_as_source"`
	}
	if e = json.Unmarshal(b, &rows); e != nil {
		return nil, e
	}
	var out []LANInterface
	for _, r := range rows {
		if r.Secondary {
			continue
		}
		cidr := r.IP + "/" + strconv.Itoa(r.Prefix)
		broadcast, e := Broadcast(cidr)
		if e == nil {
			out = append(out, LANInterface{Name: r.Name, Address: r.IP, CIDR: cidr, Broadcast: broadcast, AdapterGUID: r.AdapterGUID, Index: r.Index, Prefix: r.Prefix, Gateway: r.Gateway, DHCP: r.DHCP})
		}
	}
	return out, nil
}
func NetworkForInstallation(root string, n LANInterface, addresses map[string]string, adopt bool) (*NetworkPlan, error) {
	b, e := os.ReadFile(filepath.Join(root, "MultiServer", "config.json"))
	if e != nil {
		return nil, e
	}
	var cfg struct {
		Listen  string `json:"listen"`
		Servers []struct {
			ID      string `json:"id"`
			Backend string `json:"backend"`
			Process struct {
				WorkingDirectory string `json:"working_directory"`
			} `json:"process"`
		} `json:"servers"`
	}
	if e = json.Unmarshal(b, &cfg); e != nil {
		return nil, e
	}
	_, port, e := net.SplitHostPort(cfg.Listen)
	if e != nil {
		return nil, e
	}
	d, e := strconv.Atoi(port)
	if e != nil {
		return nil, e
	}
	p := &NetworkPlan{AdapterGUID: n.AdapterGUID, InterfaceIndex: n.Index, PrimaryIP: n.Address, PrefixLength: n.Prefix, Gateway: n.Gateway, Broadcast: n.Broadcast, DiscoveryPort: d}
	for _, s := range cfg.Servers {
		_, port, e := net.SplitHostPort(s.Backend)
		if e != nil {
			return nil, e
		}
		tcp, e := strconv.Atoi(port)
		if e != nil {
			return nil, e
		}
		b, e := os.ReadFile(filepath.Join(s.Process.WorkingDirectory, "server.properties"))
		if e != nil {
			return nil, e
		}
		props, e := ReadProperties(b)
		if e != nil {
			return nil, e
		}
		p.Servers = append(p.Servers, ServerNetwork{ID: s.ID, IP: addresses[s.ID], TCPPort: tcp, UDPRange: props["server-udp-ports"], Adopt: adopt})
	}
	return p, p.Validate()
}
