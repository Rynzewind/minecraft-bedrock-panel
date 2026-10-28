package main

import (
	"encoding/json"
	"fmt"
	"net"
	"nethernet-multi-server/internal/setup"
	"os"
	"path/filepath"
)

func (w *setupWindow) selectNetwork() {
	if w.creativeIP == nil || w.survivalIP == nil || w.networkSummary == nil {
		return
	}
	i := w.network.CurrentIndex()
	if i < 0 || i >= len(w.interfaces) {
		return
	}
	n := w.interfaces[i]
	suggested := setup.SuggestedIPs(n.Address, n.Prefix, 2)
	if len(suggested) == 2 {
		w.creativeIP.SetText(suggested[1])
		w.survivalIP.SetText(suggested[0])
	}
	w.networkSummary.SetText(fmt.Sprintf("%s | Primary %s | Gateway %s | DHCP %s | Broadcast %s | Suggestions require review", n.Name, n.CIDR, n.Gateway, n.DHCP, n.Broadcast))
}
func networkPreview(p *setup.NetworkPlan) string {
	s := fmt.Sprintf("Primary: %s/%d (retained)\r\nDiscovery: %s:%d", p.PrimaryIP, p.PrefixLength, p.Broadcast, p.DiscoveryPort)
	for _, n := range p.Servers {
		s += fmt.Sprintf("\r\n%s: %s:%d, UDP %s", n.ID, n.IP, n.TCPPort, n.UDPRange)
	}
	return s
}
func runRepair(root string) {
	w := &setupWindow{repairMode: true}
	if e := w.create(); e != nil {
		return
	}
	w.destination.SetText(root)
	w.archive.SetEnabled(false)
	w.restore.SetEnabled(false)
	w.mw.SetTitle("Repair Network Configuration — NetherNet")
	b, e := os.ReadFile(filepath.Join(root, "MultiServer", "config.json"))
	if e == nil {
		var c struct {
			Interface string `json:"interface_ip"`
			Servers   []struct {
				ID      string `json:"id"`
				Backend string `json:"backend"`
			} `json:"servers"`
		}
		if json.Unmarshal(b, &c) == nil {
			for i, n := range w.interfaces {
				if n.Address == c.Interface {
					w.network.SetCurrentIndex(i)
				}
			}
			for _, s := range c.Servers {
				ip, _, e := net.SplitHostPort(s.Backend)
				if e == nil {
					switch s.ID {
					case "creative":
						w.creativeIP.SetText(ip)
					case "survival":
						w.survivalIP.SetText(ip)
					}
				}
			}
		}
	}
	if data, e := os.ReadFile(filepath.Join(root, "MultiServer", "installation-resources.json")); e == nil {
		var saved struct {
			Startup string `json:"startup"`
		}
		if json.Unmarshal(data, &saved) == nil {
			switch saved.Startup {
			case "none":
				w.startup.SetCurrentIndex(0)
			case "logon":
				w.startup.SetCurrentIndex(1)
			case "boot":
				w.startup.SetCurrentIndex(2)
			}
		}
	}
	w.message.SetText("Review the adapter and static server IPs. Configuration creates missing secondary IPs and synchronises the helper and both server.properties files. Worlds will not be modified. Running servers stop cleanly; restart from the panel after configuration.")
	w.mw.Run()
}
