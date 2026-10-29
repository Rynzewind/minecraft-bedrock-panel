package proxy

import (
	"context"
	"crypto/elliptic"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/df-mc/go-nethernet"
	"github.com/df-mc/go-nethernet/endpoint"
	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
	"github.com/sandertv/gophertunnel/minecraft"
)

type ownedGatewayListener struct {
	authenticatedGatewayListener
	once     sync.Once
	shutdown func() error
	err      error
}

func (l *ownedGatewayListener) Close() error {
	l.once.Do(func() { l.err = l.shutdown() })
	return l.err
}

// The current client first discovers HTTP /v1/join, then negotiates a signed
// WebRTC connection. All media uses the same forwarded UDP port through ICE mux.
func listenRemoteGatewayNetherNet(ctx context.Context, address string, opts RemoteGatewayOptions, mc minecraft.ListenConfig) (_ *ownedGatewayListener, err error) {
	if opts.OperatorKey == nil || opts.OperatorKey.Curve != elliptic.P384() || !opts.PublicIPv4.Is4() || opts.PublicIPv4.IsUnspecified() {
		return nil, errors.New("persistent operator identity and gateway IPv4 address required")
	}
	id, err := opts.Server.ParsedNetworkID()
	if err != nil {
		return nil, err
	}
	verify, err := newGatewayClientVerifier(ctx, opts.Authorize)
	if err != nil {
		return nil, err
	}
	transportCtx, cancel := context.WithCancel(ctx)
	udpAddress, err := gatewayUDPAddress(address, opts.InterfaceIP)
	if err != nil {
		cancel()
		return nil, err
	}
	udp, err := net.ListenUDP("udp4", udpAddress)
	if err != nil {
		cancel()
		return nil, err
	}
	var mux *ice.UDPMuxDefault
	var tcp net.Listener
	var server *http.Server
	var listener *minecraft.Listener
	var once sync.Once
	shutdown := func() error {
		var result error
		once.Do(func() {
			cancel()
			if server != nil {
				result = errors.Join(result, server.Close())
			} else if tcp != nil {
				result = errors.Join(result, tcp.Close())
			}
			if listener != nil {
				result = errors.Join(result, listener.Close())
			}
			if mux != nil {
				result = errors.Join(result, mux.Close())
			} else {
				result = errors.Join(result, udp.Close())
			}
		})
		return result
	}
	defer func() {
		if err != nil {
			_ = shutdown()
		}
	}()
	var current atomic.Pointer[minecraft.Listener]
	pings := make(chan gatewayPing, 16)
	mux = ice.NewUDPMuxDefault(ice.UDPMuxParams{UDPConn: udp, OnUnhandledPacket: func(data []byte, addr netip.AddrPort) {
		if stamp, ok := gatewayPingTimestamp(data); ok {
			select {
			case pings <- gatewayPing{stamp: stamp, addr: addr}:
			default:
			}
		}
	}})
	go func() {
		for {
			select {
			case <-transportCtx.Done():
				return
			case ping := <-pings:
				players := 0
				if l := current.Load(); l != nil {
					players = l.PlayerCount()
				}
				status := (remoteGatewayStatus{server: opts.Server}).ServerStatus(players, 0)
				_, _ = udp.WriteToUDPAddrPort(gatewayPong(ping.stamp, id, uint16(udpAddress.Port), status), ping.addr)
			}
		}
	}()
	var settings webrtc.SettingEngine
	settings.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	settings.SetICEUDPMux(mux)
	settings.SetNAT1To1IPs([]string{opts.PublicIPv4.String()}, webrtc.ICECandidateTypeHost)
	if ip := net.ParseIP(udpAddress.IP.String()); ip != nil && ip.IsLoopback() {
		settings.SetIncludeLoopbackCandidate(true)
	}
	handler := (endpoint.HandlerConfig{Logger: mc.ErrorLog, NetworkID: strconv.FormatUint(id, 10), NegotiationContext: func(parent context.Context) (context.Context, context.CancelFunc) {
		return context.WithTimeout(parent, 15*time.Second)
	}}).New()
	network := minecraft.NetherNet{Signaling: handler, Log: mc.ErrorLog, ListenConfig: nethernet.ListenConfig{
		Log: mc.ErrorLog, API: webrtc.NewAPI(webrtc.WithSettingEngine(settings)), AllowAnonymous: false, DisableTrickleICE: true,
		VerifyClientToken: verify,
		IssueServerIdentity: func(context.Context) (*nethernet.Identity, error) {
			return nethernet.GenerateServerIdentity(opts.OperatorKey, "self")
		},
		NegotiationContext: func(parent context.Context) (context.Context, context.CancelFunc) {
			return context.WithTimeout(parent, 15*time.Second)
		},
		ConnContext: func(parent context.Context, _ *nethernet.Conn) (context.Context, context.CancelFunc) {
			return context.WithTimeout(parent, 20*time.Second)
		},
	}}
	listener, err = mc.ListenNetwork(network, strconv.FormatUint(id, 10))
	if err != nil {
		return nil, fmt.Errorf("start authenticated NetherNet gateway: %w", err)
	}
	current.Store(listener)
	// Publish discovery immediately; the protocol listener also refreshes it
	// via PongData whenever its active connection count changes.
	handler.PongData(gatewayPongData(id, uint16(udpAddress.Port), (remoteGatewayStatus{server: opts.Server}).ServerStatus(0, 0)))
	tcp, err = net.Listen("tcp4", address)
	if err != nil {
		return nil, fmt.Errorf("start gateway HTTP signaling: %w", err)
	}
	server = &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		if serveErr := server.Serve(tcp); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			if mc.ErrorLog != nil {
				mc.ErrorLog.Error("gateway signaling stopped")
			}
			_ = shutdown()
		}
	}()
	return &ownedGatewayListener{authenticatedGatewayListener: authenticatedGatewayListener{listener}, shutdown: shutdown}, nil
}

// Pion's UDP-mux gathering enumerates the mux socket, bypassing IPFilter.
// Bind the actual socket to the selected adapter instead of a wildcard.
func gatewayUDPAddress(address string, iface net.IP) (*net.UDPAddr, error) {
	a, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return nil, err
	}
	if iface != nil {
		if iface.To4() == nil || iface.IsUnspecified() || iface.IsMulticast() {
			return nil, errors.New("gateway interface must be a concrete IPv4 address")
		}
		if !a.IP.IsUnspecified() && !a.IP.Equal(iface) {
			return nil, errors.New("gateway bind address differs from selected interface")
		}
		a.IP = iface.To4()
	}
	if a.IP == nil || a.IP.IsUnspecified() {
		return nil, errors.New("gateway UDP mux requires a selected local interface")
	}
	return a, nil
}
