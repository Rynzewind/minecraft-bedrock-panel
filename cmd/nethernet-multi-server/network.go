package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"nethernet-multi-server/internal/config"
	"nethernet-multi-server/internal/networkconfig"
	"os"
	"path/filepath"
	"time"
)

func configuredNetworkAvailable(cfg config.Config) error {
	addresses, e := net.InterfaceAddrs()
	if e != nil {
		return e
	}
	local := map[string]bool{}
	for _, a := range addresses {
		ip, _, _ := net.ParseCIDR(a.String())
		local[ip.String()] = true
	}
	if !local[cfg.InterfaceIP] {
		return fmt.Errorf("configured interface IP %s is absent", cfg.InterfaceIP)
	}
	for _, s := range cfg.Servers {
		host, port, e := net.SplitHostPort(s.Backend)
		if e != nil {
			return e
		}
		if !local[host] {
			return fmt.Errorf("%s backend IP %s is absent", s.ID, host)
		}
		if s.Process != nil {
			b, e := os.ReadFile(filepath.Join(s.Process.WorkingDirectory, "server.properties"))
			if e != nil {
				return e
			}
			p, e := networkconfig.ReadProperties(b)
			if e != nil {
				return e
			}
			if p["server-ip"] != host || p["server-port"] != port || p["transport"] != "nethernet" {
				return fmt.Errorf("%s properties and backend differ", s.ID)
			}
		}
	}
	return nil
}
func waitForManagedNetwork(ctx context.Context, cfg config.Config, out io.Writer) bool {
	for _, s := range cfg.Servers {
		if s.Process == nil {
			return true
		}
	}
	probe := func() error {
		e := configuredNetworkAvailable(cfg)
		if e == nil {
			for _, s := range cfg.Servers {
				dialer := net.Dialer{Timeout: time.Second, LocalAddr: &net.TCPAddr{IP: net.ParseIP(cfg.InterfaceIP)}}
				c, err := dialer.DialContext(ctx, "tcp4", s.Backend)
				if err != nil {
					e = fmt.Errorf("%s TCP signalling is not ready: %v", s.ID, err)
					break
				}
				c.Close()
			}
		}
		return e
	}
	return waitForNetworkReadiness(ctx, probe, out, 90*time.Second, time.Second)
}
func waitForNetworkReadiness(ctx context.Context, probe func() error, out io.Writer, patience, retry time.Duration) bool {
	deadline := time.Now().Add(patience)
	warned := false
	last := ""
	for {
		if ctx.Err() != nil {
			return false
		}
		e := probe()
		if e == nil {
			return true
		}
		if e != nil && last != e.Error() {
			last = e.Error()
			fmt.Fprintf(out, "Network readiness: %s. Administration remains available; use Repair Network Configuration if the settings have changed.\n", last)
		}
		if !warned && time.Now().After(deadline) {
			warned = true
			fmt.Fprintln(out, "Initial readiness timeout; continuing to wait for the configured addresses and server endpoints. Advertising will resume automatically when ready; administration remains available.")
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(retry):
		}
	}
}
func stopAdvertisingOnDrift(cfg config.Config, closeAdvertising func()) bool {
	if configuredNetworkAvailable(cfg) != nil {
		closeAdvertising()
		return true
	}
	return false
}
