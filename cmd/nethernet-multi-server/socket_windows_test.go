//go:build windows

package main

import (
	"net"
	"syscall"
	"testing"
)

func TestListenIPv4BroadcastCreatesIPv4SocketWithBroadcastEnabled(t *testing.T) {
	interfaceIP, broadcastIP := testBroadcastInterface(t)
	conn, err := listenIPv4Broadcast("udp4", interfaceIP, net.JoinHostPort(broadcastIP, "0"))
	if err != nil {
		t.Fatalf("listenIPv4Broadcast() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	udpConn, ok := conn.(*net.UDPConn)
	if !ok {
		t.Fatalf("connection type = %T; want *net.UDPConn", conn)
	}
	addr, ok := udpConn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP.To4() == nil {
		t.Fatalf("local address = %#v; want IPv4 UDP address", udpConn.LocalAddr())
	}
	if !addr.IP.IsUnspecified() {
		t.Fatalf("local IP = %s; want IPv4 wildcard with interface filter for %s", addr.IP, interfaceIP)
	}

	raw, err := udpConn.SyscallConn()
	if err != nil {
		t.Fatalf("SyscallConn() error = %v", err)
	}
	var broadcast int
	var socketErr error
	if err := raw.Control(func(fd uintptr) {
		broadcast, socketErr = syscall.GetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST)
	}); err != nil {
		t.Fatalf("socket control error = %v", err)
	}
	if socketErr != nil {
		t.Fatalf("getsockopt SO_BROADCAST error = %v", socketErr)
	}
	if broadcast != 1 {
		t.Fatalf("SO_BROADCAST = %d; want 1", broadcast)
	}
	const ipReceiveBroadcast = 22
	var receiveBroadcast int
	if err := raw.Control(func(fd uintptr) {
		receiveBroadcast, socketErr = syscall.GetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, ipReceiveBroadcast)
	}); err != nil {
		t.Fatalf("socket control error = %v", err)
	}
	if socketErr != nil {
		t.Fatalf("getsockopt IP_RECEIVE_BROADCAST error = %v", socketErr)
	}
	if receiveBroadcast != 1 {
		t.Fatalf("IP_RECEIVE_BROADCAST = %d; want 1", receiveBroadcast)
	}
	const ipIFList = 28
	var interfaceFilter int
	if err := raw.Control(func(fd uintptr) {
		interfaceFilter, socketErr = syscall.GetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, ipIFList)
	}); err != nil {
		t.Fatalf("socket control error = %v", err)
	}
	if socketErr != nil || interfaceFilter != 1 {
		t.Fatalf("IP_IFLIST = %d, %v; want 1, nil", interfaceFilter, socketErr)
	}
}

func TestListenIPv4BroadcastRejectsWrongSubnetBroadcast(t *testing.T) {
	interfaceIP, _ := testBroadcastInterface(t)
	if _, err := listenIPv4Broadcast("udp4", interfaceIP, "203.0.113.255:0"); err == nil {
		t.Fatal("listenIPv4Broadcast() error = nil; want broadcast mismatch error")
	}
}

func testBroadcastInterface(t *testing.T) (string, string) {
	t.Helper()
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ip, network, err := net.ParseCIDR(address.String())
			if err != nil || ip.To4() == nil {
				continue
			}
			broadcast := make(net.IP, net.IPv4len)
			ipv4 := ip.To4()
			for i := range broadcast {
				broadcast[i] = ipv4[i] | ^network.Mask[i]
			}
			return ipv4.String(), broadcast.String()
		}
	}
	t.Skip("no active broadcast-capable IPv4 interface")
	return "", ""
}
