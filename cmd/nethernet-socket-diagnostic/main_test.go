//go:build windows

package main

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestDiagnosticModesCoverRequestedSocketConfigurations(t *testing.T) {
	want := []string{
		"wildcard-default",
		"wildcard-broadcast",
		"helper-current",
		"wildcard-exclusive",
		"wildcard-reuse",
		"interface-246",
		"interface-247",
		"broadcast-255",
	}
	got := diagnosticModeNames()
	if len(got) != len(want) {
		t.Fatalf("diagnosticModeNames() = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("diagnosticModeNames()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

func TestParseIPv4PacketInfo(t *testing.T) {
	oob := make([]byte, 24)
	binary.LittleEndian.PutUint64(oob[0:8], 24)
	binary.LittleEndian.PutUint32(oob[8:12], 0)
	binary.LittleEndian.PutUint32(oob[12:16], 19)
	copy(oob[16:20], net.ParseIP("192.168.1.255").To4())
	binary.LittleEndian.PutUint32(oob[20:24], 13)
	destination, interfaceIndex := parseIPv4PacketInfo(oob)
	if destination != "192.168.1.255" || interfaceIndex != 13 {
		t.Fatalf("parseIPv4PacketInfo() = %q, %d; want 192.168.1.255, 13", destination, interfaceIndex)
	}
}

func TestIsTargetPS5DiscoveryRequiresExactSourceAndLength(t *testing.T) {
	tests := []struct {
		name   string
		source *net.UDPAddr
		length int
		want   bool
	}{
		{name: "target", source: &net.UDPAddr{IP: net.ParseIP("192.168.1.125"), Port: 7551}, length: 64, want: true},
		{name: "ephemeral source port", source: &net.UDPAddr{IP: net.ParseIP("192.168.1.125"), Port: 60000}, length: 64},
		{name: "wrong source IP", source: &net.UDPAddr{IP: net.ParseIP("192.168.1.124"), Port: 7551}, length: 64},
		{name: "wrong length", source: &net.UDPAddr{IP: net.ParseIP("192.168.1.125"), Port: 7551}, length: 63},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isTargetPS5Discovery(test.source, test.length); got != test.want {
				t.Fatalf("isTargetPS5Discovery(%v, %d) = %v; want %v", test.source, test.length, got, test.want)
			}
		})
	}
}

func TestFindModeRejectsUnknownName(t *testing.T) {
	if _, err := findMode("not-a-mode"); err == nil {
		t.Fatal("findMode() error = nil; want unknown mode error")
	}
}
