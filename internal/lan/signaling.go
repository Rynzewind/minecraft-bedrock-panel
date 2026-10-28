package lan

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/df-mc/go-nethernet"
	netherdiscovery "github.com/df-mc/go-nethernet/discovery"
)

type signalingEndpoint struct {
	service   *Service
	networkID uint64
	mu        sync.RWMutex
	next      uint64
	notifiers map[uint64]nethernet.Notifier
}

func (s *Service) Signaling(networkID uint64) nethernet.Signaling {
	s.endpointsMu.Lock()
	defer s.endpointsMu.Unlock()
	if endpoint := s.endpoints[networkID]; endpoint != nil {
		return endpoint
	}
	endpoint := &signalingEndpoint{service: s, networkID: networkID, notifiers: make(map[uint64]nethernet.Notifier)}
	s.endpoints[networkID] = endpoint
	return endpoint
}

func (e *signalingEndpoint) Signal(ctx context.Context, signal *nethernet.Signal) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-e.service.ctx.Done():
		return net.ErrClosed
	default:
	}
	clientID, err := strconv.ParseUint(signal.NetworkID, 10, 64)
	if err != nil {
		return fmt.Errorf("parse remote network ID: %w", err)
	}
	e.service.clientsMu.RLock()
	addr, ok := e.service.clients[clientID]
	e.service.clientsMu.RUnlock()
	if !ok {
		return fmt.Errorf("no address found for network ID: %d", clientID)
	}
	payload := netherdiscovery.Marshal(&netherdiscovery.MessagePacket{RecipientID: clientID, Data: signal.String()}, e.networkID)
	_, err = e.service.conn.WriteTo(payload, addr)
	return err
}

func (e *signalingEndpoint) Notify(n nethernet.Notifier) func() {
	if n == nil {
		panic("lan: nil signaling notifier")
	}
	e.mu.Lock()
	id := e.next
	e.next++
	e.notifiers[id] = n
	e.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { e.mu.Lock(); delete(e.notifiers, id); e.mu.Unlock() }) }
}

func (e *signalingEndpoint) Context() context.Context { return e.service.ctx }
func (e *signalingEndpoint) Credentials(context.Context) (*nethernet.Credentials, error) {
	return nil, nil
}
func (e *signalingEndpoint) NetworkID() string { return strconv.FormatUint(e.networkID, 10) }
func (*signalingEndpoint) PongData([]byte)     {}

func (e *signalingEndpoint) notify(signal *nethernet.Signal) {
	e.mu.RLock()
	list := make([]nethernet.Notifier, 0, len(e.notifiers))
	for _, notifier := range e.notifiers {
		list = append(list, notifier)
	}
	e.mu.RUnlock()
	for _, notifier := range list {
		notifier.NotifySignal(signal)
	}
}

func (s *Service) dispatchMessage(senderID uint64, packet *netherdiscovery.MessagePacket) error {
	if packet.Data == "" || packet.Data == "Ping" {
		return nil
	}
	s.endpointsMu.RLock()
	endpoint := s.endpoints[packet.RecipientID]
	s.endpointsMu.RUnlock()
	if endpoint == nil {
		return fmt.Errorf("unknown signalling recipient network ID: %d", packet.RecipientID)
	}
	signal := &nethernet.Signal{}
	if err := signal.UnmarshalText([]byte(packet.Data)); err != nil {
		return fmt.Errorf("decode NetherNet signal: %w", err)
	}
	signal.NetworkID = strconv.FormatUint(senderID, 10)
	endpoint.notify(signal)
	return nil
}

var _ nethernet.Signaling = (*signalingEndpoint)(nil)
