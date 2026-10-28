package networkconfig

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type ServerNetwork struct {
	ID       string `json:"id"`
	IP       string `json:"ip"`
	TCPPort  int    `json:"tcp_port"`
	UDPRange string `json:"udp_range"`
	Adopt    bool   `json:"adopt"`
}
type NetworkPlan struct {
	AdapterGUID    string          `json:"adapter_guid"`
	InterfaceIndex int             `json:"interface_index"`
	PrimaryIP      string          `json:"primary_ip"`
	PrefixLength   int             `json:"prefix_length"`
	Gateway        string          `json:"gateway"`
	Broadcast      string          `json:"broadcast"`
	DiscoveryPort  int             `json:"discovery_port"`
	Servers        []ServerNetwork `json:"servers"`
}

func UDPPorts(s string) (int, int, error) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, 0, errors.New("UDP range must be first-last")
	}
	a, e := strconv.Atoi(parts[0])
	if e != nil {
		return 0, 0, e
	}
	b, e := strconv.Atoi(parts[1])
	if e != nil || a < 1 || b > 65535 || a > b {
		return 0, 0, errors.New("invalid UDP range")
	}
	return a, b, nil
}
func (p *NetworkPlan) Validate() error {
	ip := net.ParseIP(p.PrimaryIP).To4()
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsLinkLocalUnicast() || p.PrefixLength < 1 || p.PrefixLength > 30 || p.InterfaceIndex < 1 || p.AdapterGUID == "" {
		return errors.New("select an active LAN adapter and primary IPv4 address with a usable subnet")
	}
	n := &net.IPNet{IP: ip.Mask(net.CIDRMask(p.PrefixLength, 32)), Mask: net.CIDRMask(p.PrefixLength, 32)}
	b, _ := Broadcast(fmt.Sprintf("%s/%d", p.PrimaryIP, p.PrefixLength))
	if p.Broadcast != "" && p.Broadcast != b {
		return errors.New("broadcast does not match the selected subnet")
	}
	p.Broadcast = b
	if p.DiscoveryPort < 1 || p.DiscoveryPort > 65535 {
		return errors.New("invalid discovery port")
	}
	seen := map[string]bool{}
	ids := map[string]bool{}
	for i, s := range p.Servers {
		a := net.ParseIP(s.IP).To4()
		if a == nil || !n.Contains(a) || a.Equal(n.IP) || s.IP == b || a.Equal(ip) || s.IP == p.Gateway || seen[s.IP] || s.ID == "" || ids[s.ID] {
			return fmt.Errorf("server %s needs a distinct usable secondary IP in %s", s.ID, n)
		}
		if s.TCPPort < 1 || s.TCPPort > 65535 {
			return errors.New("invalid TCP port")
		}
		lo, hi, e := UDPPorts(s.UDPRange)
		if e != nil {
			return fmt.Errorf("%s: %w", s.ID, e)
		}
		if lo <= p.DiscoveryPort && p.DiscoveryPort <= hi {
			return errors.New("gameplay UDP range overlaps discovery")
		}
		for _, prev := range p.Servers[:i] {
			a, b, _ := UDPPorts(prev.UDPRange)
			if lo <= b && a <= hi {
				return errors.New("gameplay UDP ranges overlap")
			}
		}
		seen[s.IP] = true
		ids[s.ID] = true
	}
	if len(p.Servers) == 0 {
		return errors.New("no managed servers")
	}
	return nil
}
func SuggestedIPs(primary string, prefix int, count int) []string {
	ip := net.ParseIP(primary).To4()
	if ip == nil || prefix < 1 || prefix > 30 {
		return nil
	}
	mask := net.CIDRMask(prefix, 32)
	base := binary.BigEndian.Uint32(ip.Mask(mask))
	last := base | ^binary.BigEndian.Uint32(mask)
	var out []string
	for a := last - 1; a > base && len(out) < count; a-- {
		b := make(net.IP, 4)
		binary.BigEndian.PutUint32(b, a)
		if !b.Equal(ip) {
			out = append(out, b.String())
		}
	}
	return out
}
