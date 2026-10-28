package lan

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	netherdiscovery "github.com/df-mc/go-nethernet/discovery"
)

const maxUDPPacketSize = 65535

type Advertisement struct {
	Name       string
	Backend    string
	NetworkID  uint64
	ServerData netherdiscovery.ServerData
}

type Service struct {
	conn           net.PacketConn
	advertisements []Advertisement
	log            *slog.Logger
	ctx            context.Context
	cancel         context.CancelFunc
	endpoints      map[uint64]*signalingEndpoint
	endpointsMu    sync.RWMutex
	clients        map[uint64]net.Addr
	clientsMu      sync.RWMutex
}

func New(conn net.PacketConn, advertisements []Advertisement, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		conn:           conn,
		advertisements: append([]Advertisement(nil), advertisements...),
		log:            logger,
		ctx:            ctx,
		cancel:         cancel,
		endpoints:      make(map[uint64]*signalingEndpoint),
		clients:        make(map[uint64]net.Addr),
	}
}

func (s *Service) Serve(ctx context.Context) error {
	defer s.cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = s.conn.Close()
		case <-done:
		}
	}()

	buf := make([]byte, maxUDPPacketSize)
	for {
		n, addr, err := s.conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read discovery datagram: %w", err)
		}
		s.log.Debug("UDP datagram received",
			"from", addr.String(),
			"bytes", n,
		)
		if err := s.handleDatagram(buf[:n], addr); err != nil {
			s.log.Warn("rejected discovery datagram", "from", addr.String(), "error", err)
		}
	}
}

func (s *Service) handleDatagram(datagram []byte, addr net.Addr) error {
	packet, senderID, err := netherdiscovery.Unmarshal(datagram)
	if err != nil {
		return fmt.Errorf("decode discovery datagram: %w", err)
	}
	s.log.Info("discovery packet received", "from", addr.String(), "sender_network_id", senderID, "packet_type", fmt.Sprintf("%T", packet))
	if senderID != 0 {
		s.clientsMu.Lock()
		s.clients[senderID] = addr
		s.clientsMu.Unlock()
	}

	switch typed := packet.(type) {
	case *netherdiscovery.MessagePacket:
		s.log.Info("signalling message observed",
			"from", addr.String(),
			"sender_network_id", senderID,
			"recipient_network_id", typed.RecipientID,
			"signal_bytes", len(typed.Data),
		)
		return s.dispatchMessage(senderID, typed)
	case *netherdiscovery.ResponsePacket:
		s.log.Info("other LAN advertisement observed", "from", addr.String(), "sender_network_id", senderID, "application_data_bytes", len(typed.ApplicationData))
		return nil
	case *netherdiscovery.RequestPacket:
		// Respond below.
	default:
		return nil
	}
	for _, advertisement := range s.advertisements {
		applicationData, err := advertisement.ServerData.MarshalBinary()
		if err != nil {
			return fmt.Errorf("marshal advertisement %q: %w", advertisement.Name, err)
		}
		payload := netherdiscovery.Marshal(&netherdiscovery.ResponsePacket{ApplicationData: applicationData}, advertisement.NetworkID)
		n, err := s.conn.WriteTo(payload, addr)
		if err != nil {
			s.log.Error("advertisement send failed", "to", addr.String(), "server", advertisement.Name, "backend", advertisement.Backend, "network_id", advertisement.NetworkID, "error", err)
			continue
		}
		s.log.Info("advertisement sent", "to", addr.String(), "server", advertisement.Name, "backend", advertisement.Backend, "network_id", advertisement.NetworkID, "bytes", n)
	}
	return nil
}
