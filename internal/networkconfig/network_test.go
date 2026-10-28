package networkconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNetworkPlanUsesRealSubnetAndRejectsUnsafeAddresses(t *testing.T) {
	p := NetworkPlan{AdapterGUID: "adapter", InterfaceIndex: 9, PrimaryIP: "10.42.8.9", PrefixLength: 20, Gateway: "10.42.8.1", Servers: []ServerNetwork{{ID: "creative", IP: "10.42.9.246", TCPPort: 19134, UDPRange: "49152-49200"}, {ID: "survival", IP: "10.42.9.247", TCPPort: 19136, UDPRange: "49201-49249"}}, DiscoveryPort: 7551}
	if e := p.Validate(); e != nil {
		t.Fatal(e)
	}
	if p.Broadcast != "10.42.15.255" {
		t.Fatal(p.Broadcast)
	}
	for _, ip := range []string{"10.42.8.9", "10.42.8.1", "10.42.0.0", "10.42.15.255", "192.168.1.246"} {
		q := p
		q.Servers = append([]ServerNetwork(nil), p.Servers...)
		q.Servers[0].IP = ip
		if q.Validate() == nil {
			t.Fatal("accepted unsafe IP", ip)
		}
	}
	q := p
	q.Servers = append([]ServerNetwork(nil), p.Servers...)
	q.Servers[1].UDPRange = "49200-49249"
	if q.Validate() == nil {
		t.Fatal("overlap accepted")
	}
}
func TestConfigurationPreservesUnknownJSONAndProperties(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "MultiServer"), 0700)
	os.MkdirAll(filepath.Join(root, "Creative"), 0700)
	cfg := map[string]any{"interface_ip": "192.168.1.137", "listen": "192.168.1.255:7551", "future": map[string]any{"keep": true}, "servers": []any{map[string]any{"id": "creative", "backend": "192.168.1.137:19134", "network_id": "permanent", "extra": "retain", "process": map[string]string{"working_directory": filepath.Join(root, "Creative")}}}}
	b, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(root, "MultiServer", "config.json"), b, 0600)
	props := "# keep this\r\ntransport=nethernet\r\nserver-ip=192.168.1.137\r\nserver-port=19134\r\nserver-udp-ports=49152-49200\r\nenable-lan-visibility=false\r\nlevel-name=My World\r\n"
	os.WriteFile(filepath.Join(root, "Creative", "server.properties"), []byte(props), 0600)
	p := NetworkPlan{AdapterGUID: "a", InterfaceIndex: 9, PrimaryIP: "10.42.8.9", PrefixLength: 20, DiscoveryPort: 7551, Servers: []ServerNetwork{{ID: "creative", IP: "10.42.9.246", TCPPort: 19134, UDPRange: "49152-49200"}}}
	changes, e := PrepareNetworkConfiguration(root, &p)
	if e != nil {
		t.Fatal(e)
	}
	if len(changes) != 2 {
		t.Fatal(len(changes))
	}
	if !strings.Contains(changes[0].Content, "\"future\"") || !strings.Contains(changes[0].Content, "permanent") || !strings.Contains(changes[0].Content, "10.42.9.246:19134") {
		t.Fatal(changes[0].Content)
	}
	want := strings.Replace(props, "server-ip=192.168.1.137", "server-ip=10.42.9.246", 1)
	if changes[1].Content != want {
		t.Fatal("unrelated properties changed", changes[1].Content)
	}
	os.WriteFile(filepath.Join(root, "Creative", "server.properties"), []byte(props+"server-ip=duplicate\r\n"), 0600)
	if _, e = PrepareNetworkConfiguration(root, &p); e == nil {
		t.Fatal("duplicate key accepted")
	}
}
