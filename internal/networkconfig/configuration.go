package networkconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

type ConfigurationChange struct {
	RelativePath   string `json:"relative_path"`
	OriginalSHA256 string `json:"original_sha256"`
	Content        string `json:"content"`
}

func ReadProperties(b []byte) (map[string]string, error) {
	out := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if _, ok := out[k]; ok {
			return nil, fmt.Errorf("duplicate property %s", k)
		}
		out[k] = strings.TrimSpace(v)
	}
	return out, nil
}
func patchProperty(b []byte, key, value string) ([]byte, error) {
	props, e := ReadProperties(b)
	if e != nil {
		return nil, e
	}
	text := string(b)
	newline := "\n"
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(text, newline)
	found := false
	for i, line := range lines {
		k, _, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == key {
			lines[i] = key + "=" + value
			found = true
		}
	}
	if _, ok := props[key]; !ok || !found {
		if text != "" && !strings.HasSuffix(text, newline) {
			text += newline
		}
		return []byte(text + key + "=" + value + newline), nil
	}
	return []byte(strings.Join(lines, newline)), nil
}
func PrepareNetworkConfiguration(root string, p *NetworkPlan) ([]ConfigurationChange, error) {
	if e := p.Validate(); e != nil {
		return nil, e
	}
	b, e := os.ReadFile(filepath.Join(root, "MultiServer", "config.json"))
	if e != nil {
		return nil, e
	}
	var cfg map[string]json.RawMessage
	if e = json.Unmarshal(b, &cfg); e != nil {
		return nil, e
	}
	var servers []map[string]json.RawMessage
	if e = json.Unmarshal(cfg["servers"], &servers); e != nil {
		return nil, e
	}
	if len(servers) != len(p.Servers) {
		return nil, errors.New("network plan must include every server")
	}
	changes := []ConfigurationChange{{RelativePath: "MultiServer/config.json"}}
	hash := sha256.Sum256(b)
	changes[0].OriginalSHA256 = hex.EncodeToString(hash[:])
	seen := map[string]bool{}
	for _, s := range servers {
		var id, backend string
		json.Unmarshal(s["id"], &id)
		json.Unmarshal(s["backend"], &backend)
		var selected *ServerNetwork
		for i := range p.Servers {
			if p.Servers[i].ID == id {
				selected = &p.Servers[i]
			}
		}
		if selected == nil || seen[id] {
			return nil, errors.New("server ID is missing or ambiguous")
		}
		seen[id] = true
		_, port, e := net.SplitHostPort(backend)
		if e != nil || port != fmt.Sprint(selected.TCPPort) {
			return nil, errors.New("existing backend TCP port must be preserved")
		}
		var process struct {
			WorkingDirectory string `json:"working_directory"`
		}
		if e = json.Unmarshal(s["process"], &process); e != nil {
			return nil, e
		}
		rel, e := filepath.Rel(root, process.WorkingDirectory)
		if e != nil || !filepath.IsLocal(rel) || rel == "." {
			return nil, errors.New("server working directory is outside installation")
		}
		path := filepath.Join(process.WorkingDirectory, "server.properties")
		props, e := os.ReadFile(path)
		if e != nil {
			return nil, e
		}
		values, e := ReadProperties(props)
		if e != nil {
			return nil, e
		}
		if values["transport"] != "nethernet" || values["server-port"] != port || values["server-udp-ports"] != selected.UDPRange {
			return nil, errors.New("server transport/ports differ from reviewed plan; repair those settings explicitly first")
		}
		updated, e := patchProperty(props, "server-ip", selected.IP)
		if e != nil {
			return nil, e
		}
		updated, e = patchProperty(updated, "enable-lan-visibility", "false")
		if e != nil {
			return nil, e
		}
		h := sha256.Sum256(props)
		changes = append(changes, ConfigurationChange{filepath.ToSlash(filepath.Join(rel, "server.properties")), hex.EncodeToString(h[:]), string(updated)})
		s["backend"], _ = json.Marshal(net.JoinHostPort(selected.IP, port))
	}
	cfg["interface_ip"], _ = json.Marshal(p.PrimaryIP)
	cfg["listen"], _ = json.Marshal(net.JoinHostPort(p.Broadcast, fmt.Sprint(p.DiscoveryPort)))
	cfg["servers"], _ = json.Marshal(servers)
	updated, e := json.MarshalIndent(cfg, "", "  ")
	if e != nil {
		return nil, e
	}
	changes[0].Content = string(updated)
	return changes, nil
}
