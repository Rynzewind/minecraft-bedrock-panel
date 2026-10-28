package networkconfig

import (
	"errors"
	"net"
)

func Broadcast(cidr string) (string, error) {
	ip, n, e := net.ParseCIDR(cidr)
	if e != nil || ip.To4() == nil {
		return "", errors.New("not IPv4")
	}
	ones, bits := n.Mask.Size()
	if bits != 32 || ones > 30 {
		return "", errors.New("LAN subnet must have a broadcast address")
	}
	a := ip.To4()
	for i := range a {
		a[i] |= ^n.Mask[i]
	}
	return a.String(), nil
}
