package proxy

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/netip"

	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

var gatewayRakNetMagic = []byte{0, 255, 255, 0, 254, 254, 254, 254, 253, 253, 253, 253, 18, 52, 86, 120}

type gatewayPing struct {
	stamp uint64
	addr  netip.AddrPort
}

// Only status pings are answered. Legacy RakNet game sessions are not accepted.
func gatewayPingTimestamp(data []byte) (uint64, bool) {
	if len(data) != 33 || (data[0] != 1 && data[0] != 2) || !bytes.Equal(data[9:25], gatewayRakNetMagic) {
		return 0, false
	}
	return binary.BigEndian.Uint64(data[1:9]), true
}
func gatewayPongData(id uint64, port uint16, s minecraft.ServerStatus) []byte {
	mode := "Survival"
	switch s.GameType {
	case packet.GameTypeCreative:
		mode = "Creative"
	case packet.GameTypeAdventure:
		mode = "Adventure"
	}
	return []byte(fmt.Sprintf("MCPE;%s;%d;%s;%d;%d;%d;%s;%s;1;%d;%d;0;0;", s.ServerName, protocol.CurrentProtocol, protocol.CurrentVersion, s.PlayerCount, s.MaxPlayers, id, s.ServerSubName, mode, port, port))
}
func gatewayPong(stamp, id uint64, port uint16, s minecraft.ServerStatus) []byte {
	data := gatewayPongData(id, port, s)
	out := make([]byte, 35+len(data))
	out[0] = 28
	binary.BigEndian.PutUint64(out[1:9], stamp)
	binary.BigEndian.PutUint64(out[9:17], id)
	copy(out[17:33], gatewayRakNetMagic)
	binary.BigEndian.PutUint16(out[33:35], uint16(len(data)))
	copy(out[35:], data)
	return out
}
