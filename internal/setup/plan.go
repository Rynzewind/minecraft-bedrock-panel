package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"nethernet-multi-server/internal/config"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Options struct {
	Root, UserSID, Startup, RuntimeInstaller string
	OldDeploymentStopped                     bool
	Network                                  *NetworkPlan
}
type Plan struct {
	Root             string                `json:"root"`
	UserSID          string                `json:"user_sid"`
	Startup          string                `json:"startup"`
	RuntimeInstaller string                `json:"runtime_installer"`
	Group            string                `json:"group"`
	FirewallPrograms []string              `json:"firewall_programs"`
	Network          *NetworkPlan          `json:"network,omitempty"`
	Configuration    []ConfigurationChange `json:"configuration,omitempty"`
}

func BuildPlan(o Options, cfg config.Config) (Plan, error) {
	p := Plan{Root: o.Root, UserSID: o.UserSID, Startup: o.Startup, RuntimeInstaller: o.RuntimeInstaller}
	if !o.OldDeploymentStopped {
		return p, errors.New("confirm the old deployment is stopped before activation")
	}
	if !filepath.IsAbs(o.Root) || !regexp.MustCompile(`^S-1-5-(\d+-)*\d+$`).MatchString(o.UserSID) {
		return p, errors.New("invalid installation folder or Windows user")
	}
	switch o.Startup {
	case "none", "logon", "boot":
	default:
		return p, errors.New("unknown startup mode")
	}
	hash := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(o.Root))))
	p.Group = "NetherNet-Migration-" + hex.EncodeToString(hash[:6])
	p.FirewallPrograms = []string{filepath.Join(o.Root, "MultiServer", "NetherNetMultiServer.exe")}
	for _, s := range cfg.Servers {
		if s.Process == nil {
			return p, errors.New("unmanaged server")
		}
		r, e := filepath.Rel(o.Root, s.Process.Executable)
		if e != nil || !filepath.IsLocal(r) {
			return p, errors.New("firewall executable outside installation")
		}
		p.FirewallPrograms = append(p.FirewallPrograms, s.Process.Executable)
	}
	if o.Network != nil {
		p.Network = o.Network
		var e error
		p.Configuration, e = PrepareNetworkConfiguration(o.Root, p.Network)
		if e != nil {
			return p, e
		}
	}
	return p, nil
}

type Platform interface {
	DisableStartup(context.Context, Plan) error
	EnsureRuntime(context.Context, Plan) error
	ConfigureFirewall(context.Context, Plan) error
	RegisterStartup(context.Context, Plan) error
	EnableStartup(context.Context, Plan) error
}

func Apply(ctx context.Context, p Plan, platform Platform) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	gate := filepath.Join(p.Root, "migration-activation-required")
	if err = os.WriteFile(gate, []byte("Setup pending.\n"), 0600); err != nil {
		return err
	}
	if err = platform.DisableStartup(ctx, p); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = platform.DisableStartup(context.Background(), p)
		}
	}()
	for _, step := range []func(context.Context, Plan) error{platform.EnsureRuntime, platform.ConfigureFirewall, platform.RegisterStartup, platform.EnableStartup} {
		if err = step(ctx, p); err != nil {
			return err
		}
	}
	if err = os.Remove(gate); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

type LANInterface struct {
	Name, Address, CIDR, Broadcast, AdapterGUID, Gateway, DHCP string
	Index, Prefix                                              int
}

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
func Interfaces() []LANInterface {
	if detected, e := DetectInterfaces(); e == nil && len(detected) > 0 {
		return detected
	}
	var out []LANInterface
	list, _ := net.Interfaces()
	for _, iface := range list {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ip, _, e := net.ParseCIDR(a.String())
			if e != nil || ip.To4() == nil || ip.IsLinkLocalUnicast() {
				continue
			}
			broadcast, e := Broadcast(a.String())
			if e != nil {
				continue
			}
			out = append(out, LANInterface{Name: iface.Name, Address: ip.String(), CIDR: a.String(), Broadcast: broadcast})
		}
	}
	return out
}
