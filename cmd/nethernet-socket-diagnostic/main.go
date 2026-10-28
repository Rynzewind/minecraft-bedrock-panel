//go:build windows

package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
)

const (
	diagnosticPort     = 7551
	ps5Address         = "192.168.1.125"
	interface246       = "192.168.1.246"
	interface247       = "192.168.1.247"
	broadcastAddress   = "192.168.1.255"
	soExclusiveAddrUse = -5
	ipPacketInfo       = 19
	ipReceiveBroadcast = 22
	ipIFList           = 28
	ipAddIFList        = 29
)

type socketMode struct {
	name        string
	description string
	bindIP      string
	reuse       bool
	exclusive   bool
	broadcast   bool
	filterIP    string
}

var diagnosticModes = []socketMode{
	{name: "wildcard-default", description: "Go/Winsock defaults, INADDR_ANY", bindIP: "0.0.0.0"},
	{name: "wildcard-broadcast", description: "INADDR_ANY with SO_BROADCAST and IP_RECEIVE_BROADCAST", bindIP: "0.0.0.0", broadcast: true},
	{name: "helper-current", description: "current helper socket: broadcast reception restricted to Ethernet 192.168.1.246 interface", bindIP: "0.0.0.0", broadcast: true, filterIP: interface246},
	{name: "wildcard-exclusive", description: "INADDR_ANY with broadcast reception and SO_EXCLUSIVEADDRUSE", bindIP: "0.0.0.0", exclusive: true, broadcast: true},
	{name: "wildcard-reuse", description: "INADDR_ANY with broadcast reception and SO_REUSEADDR", bindIP: "0.0.0.0", reuse: true, broadcast: true},
	{name: "interface-246", description: "specific Ethernet address 192.168.1.246 with broadcast reception", bindIP: interface246, broadcast: true},
	{name: "interface-247", description: "specific Ethernet address 192.168.1.247 with broadcast reception", bindIP: interface247, broadcast: true},
	{name: "broadcast-255", description: "direct subnet broadcast bind with SO_BROADCAST", bindIP: broadcastAddress, broadcast: true},
}

func diagnosticModeNames() []string {
	names := make([]string, len(diagnosticModes))
	for i, mode := range diagnosticModes {
		names[i] = mode.name
	}
	return names
}

func findMode(name string) (socketMode, error) {
	for _, mode := range diagnosticModes {
		if mode.name == name {
			return mode, nil
		}
	}
	return socketMode{}, fmt.Errorf("unknown mode %q (want %s)", name, strings.Join(diagnosticModeNames(), ", "))
}

func isTargetPS5Discovery(source *net.UDPAddr, length int) bool {
	return source != nil && source.IP.Equal(net.ParseIP(ps5Address)) && source.Port == diagnosticPort && length == 64
}

func main() {
	modeName := flag.String("mode", "wildcard-default", "socket configuration to test")
	duration := flag.Duration("duration", 20*time.Second, "how long to receive packets")
	flag.Parse()

	mode, err := findMode(*modeName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(2)
	}
	if *duration <= 0 {
		fmt.Fprintln(os.Stderr, "ERROR: duration must be positive")
		os.Exit(2)
	}
	if err := runDiagnostic(mode, *duration); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func runDiagnostic(mode socketMode, duration time.Duration) error {
	fmt.Println("============================================================")
	fmt.Println(" NetherNet Windows UDP Socket Diagnostic")
	fmt.Println("============================================================")
	fmt.Printf("CONFIGURATION: %s\n", mode.name)
	fmt.Printf("DESCRIPTION:   %s\n", mode.description)
	fmt.Printf("BIND REQUEST:  udp4 %s:%d\n", mode.bindIP, diagnosticPort)
	fmt.Printf("OPTIONS:       SO_REUSEADDR=%t SO_EXCLUSIVEADDRUSE=%t SO_BROADCAST=%t IP_RECEIVE_BROADCAST=%t IP_PKTINFO=true IP_IFLIST=%q\n", mode.reuse, mode.exclusive, mode.broadcast, mode.broadcast, mode.filterIP)
	fmt.Printf("TARGET:        %s:%d -> broadcast:%d, 64 bytes\n", ps5Address, diagnosticPort, diagnosticPort)
	fmt.Printf("TEST WINDOW:   %s\n", duration)
	fmt.Println("------------------------------------------------------------")

	interfaceIndex, err := interfaceIndexForIP(mode.filterIP)
	if err != nil {
		return err
	}
	listenConfig := net.ListenConfig{Control: controlForMode(mode, interfaceIndex)}
	conn, err := listenConfig.ListenPacket(context.Background(), "udp4", net.JoinHostPort(mode.bindIP, fmt.Sprint(diagnosticPort)))
	if err != nil {
		return fmt.Errorf("configuration %s bind failed: %w", mode.name, err)
	}
	defer conn.Close()
	fmt.Printf("BOUND:         %s (%s)\n", conn.LocalAddr(), conn.LocalAddr().Network())

	udpConn, ok := conn.(*net.UDPConn)
	if !ok {
		return fmt.Errorf("configuration %s returned unexpected connection type %T", mode.name, conn)
	}
	deadline := time.Now().Add(duration)
	if err := conn.SetReadDeadline(deadline); err != nil {
		return fmt.Errorf("set receive deadline: %w", err)
	}
	fmt.Println("STATUS:        LISTENING — open Minecraft → Play → Worlds on PS5 now")

	targetReceived := false
	packetCount := 0
	buffer := make([]byte, 65535)
	oob := make([]byte, 128)
	for {
		n, oobn, _, sourceUDP, err := udpConn.ReadMsgUDP(buffer, oob)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				break
			}
			return fmt.Errorf("receive in configuration %s: %w", mode.name, err)
		}
		packetCount++
		isTarget := isTargetPS5Discovery(sourceUDP, n)
		if isTarget {
			targetReceived = true
		}
		destination, interfaceIndex := parseIPv4PacketInfo(oob[:oobn])
		fmt.Println()
		fmt.Println("******************** PACKET RECEIVED ********************")
		fmt.Printf("configuration=%s timestamp=%s\n", mode.name, time.Now().Format(time.RFC3339Nano))
		fmt.Printf("source_ip=%s source_port=%d destination_ip=%s interface_index=%d bytes=%d\n", sourceUDP.IP, sourceUDP.Port, destination, interfaceIndex, n)
		fmt.Printf("target_ps5_discovery=%t\n", isTarget)
		fmt.Printf("payload_hex=%s\n", hex.EncodeToString(buffer[:n]))
		if isTarget {
			fmt.Println("*** SUCCESS: ACTUAL 64-BYTE PS5 DISCOVERY DATAGRAM RECEIVED ***")
		}
		fmt.Println("*********************************************************")
	}

	fmt.Println()
	fmt.Println("======================= RESULT ==========================")
	fmt.Printf("CONFIGURATION: %s\n", mode.name)
	fmt.Printf("PACKETS SEEN:  %d\n", packetCount)
	if targetReceived {
		fmt.Printf("PS5 %s:%d 64-BYTE DATAGRAM RECEIVED: YES\n", ps5Address, diagnosticPort)
	} else {
		fmt.Printf("PS5 %s:%d 64-BYTE DATAGRAM RECEIVED: NO\n", ps5Address, diagnosticPort)
	}
	fmt.Println("=========================================================")
	return nil
}

func controlForMode(mode socketMode, interfaceIndex int) func(string, string, syscall.RawConn) error {
	return func(_, _ string, raw syscall.RawConn) error {
		var socketErr error
		if err := raw.Control(func(fd uintptr) {
			handle := syscall.Handle(fd)
			if mode.reuse {
				socketErr = syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			}
			if socketErr == nil && mode.exclusive {
				socketErr = syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, soExclusiveAddrUse, 1)
			}
			if socketErr == nil && mode.broadcast {
				socketErr = syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
			}
			if socketErr == nil && mode.broadcast {
				socketErr = syscall.SetsockoptInt(handle, syscall.IPPROTO_IP, ipReceiveBroadcast, 1)
			}
			if socketErr == nil {
				socketErr = syscall.SetsockoptInt(handle, syscall.IPPROTO_IP, ipPacketInfo, 1)
			}
			if socketErr == nil && interfaceIndex != 0 {
				socketErr = syscall.SetsockoptInt(handle, syscall.IPPROTO_IP, ipIFList, 1)
			}
			if socketErr == nil && interfaceIndex != 0 {
				socketErr = syscall.SetsockoptInt(handle, syscall.IPPROTO_IP, ipAddIFList, interfaceIndex)
			}
		}); err != nil {
			return err
		}
		return socketErr
	}
}

func interfaceIndexForIP(address string) (int, error) {
	if address == "" {
		return 0, nil
	}
	wanted := net.ParseIP(address)
	interfaces, err := net.Interfaces()
	if err != nil {
		return 0, err
	}
	for _, iface := range interfaces {
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, candidate := range addresses {
			ip, _, err := net.ParseCIDR(candidate.String())
			if err == nil && ip.Equal(wanted) {
				return iface.Index, nil
			}
		}
	}
	return 0, fmt.Errorf("no interface contains IPv4 address %s", address)
}

func parseIPv4PacketInfo(oob []byte) (string, int) {
	for offset := 0; offset+16 <= len(oob); {
		length := int(binary.LittleEndian.Uint64(oob[offset : offset+8]))
		if length < 16 || offset+length > len(oob) {
			break
		}
		level := int32(binary.LittleEndian.Uint32(oob[offset+8 : offset+12]))
		messageType := int32(binary.LittleEndian.Uint32(oob[offset+12 : offset+16]))
		if level == syscall.IPPROTO_IP && messageType == ipPacketInfo && length >= 24 {
			destination := net.IP(oob[offset+16 : offset+20]).String()
			interfaceIndex := int(binary.LittleEndian.Uint32(oob[offset+20 : offset+24]))
			return destination, interfaceIndex
		}
		offset += (length + 7) &^ 7
	}
	return "unavailable", 0
}
