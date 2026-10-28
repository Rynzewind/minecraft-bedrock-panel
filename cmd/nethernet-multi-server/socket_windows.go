//go:build windows

package main

import (
	"context"
	"fmt"
	"net"
	"syscall"
)

const ipReceiveBroadcast = 22
const ipIFList = 28
const ipAddIFList = 29

func listenIPv4Broadcast(network, interfaceIP, address string) (net.PacketConn, error) {
	if network != "udp4" {
		return nil, fmt.Errorf("unsupported discovery network %q: want udp4", network)
	}
	bindAddress, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return nil, fmt.Errorf("resolve broadcast bind address: %w", err)
	}
	interfaceAddress, expectedBroadcast, interfaceIndex, err := findInterfaceBroadcast(interfaceIP)
	if err != nil {
		return nil, err
	}
	if !bindAddress.IP.Equal(expectedBroadcast) {
		return nil, fmt.Errorf("listen address %s is not the subnet broadcast %s for interface %s", bindAddress.IP, expectedBroadcast, interfaceAddress)
	}
	wildcardBindAddress := &net.UDPAddr{IP: net.IPv4zero, Port: bindAddress.Port}

	listenConfig := net.ListenConfig{
		Control: func(_, _ string, raw syscall.RawConn) error {
			var socketErr error
			if err := raw.Control(func(fd uintptr) {
				if err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1); err != nil {
					socketErr = fmt.Errorf("SO_BROADCAST: %w", err)
					return
				}
				if err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, ipReceiveBroadcast, 1); err != nil {
					socketErr = fmt.Errorf("IP_RECEIVE_BROADCAST: %w", err)
					return
				}
				if err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, ipIFList, 1); err != nil {
					socketErr = fmt.Errorf("IP_IFLIST: %w", err)
					return
				}
				if err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, ipAddIFList, interfaceIndex); err != nil {
					socketErr = fmt.Errorf("IP_ADD_IFLIST: %w", err)
				}
			}); err != nil {
				return fmt.Errorf("access discovery socket: %w", err)
			}
			if socketErr != nil {
				return fmt.Errorf("enable Windows broadcast socket options: %w", socketErr)
			}
			return nil
		},
	}
	conn, err := listenConfig.ListenPacket(context.Background(), "udp4", wildcardBindAddress.String())
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func findInterfaceBroadcast(interfaceIP string) (net.IP, net.IP, int, error) {
	wanted := net.ParseIP(interfaceIP).To4()
	if wanted == nil {
		return nil, nil, 0, fmt.Errorf("interface IP %q is not IPv4", interfaceIP)
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, 0, fmt.Errorf("enumerate network interfaces: %w", err)
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ip, ipNetwork, err := net.ParseCIDR(address.String())
			if err != nil || ip.To4() == nil || !ip.Equal(wanted) {
				continue
			}
			ipv4 := ip.To4()
			broadcast := make(net.IP, net.IPv4len)
			for i := range broadcast {
				broadcast[i] = ipv4[i] | ^ipNetwork.Mask[i]
			}
			return ipv4, broadcast, iface.Index, nil
		}
	}
	return nil, nil, 0, fmt.Errorf("active broadcast-capable IPv4 interface %s was not found", wanted)
}
