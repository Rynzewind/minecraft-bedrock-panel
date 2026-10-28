package main

import (
	"context"

	"fmt"

	"encoding/json"
	"nethernet-multi-server/internal/accountstore"
	"nethernet-multi-server/internal/config"
	"nethernet-multi-server/internal/migration"
	"nethernet-multi-server/internal/networkconfig"

	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
	"time"
)

func main() { os.Exit(command(os.Args[1:])) }
func command(args []string) int {
	ctx := context.Background()
	if len(args) == 4 && args[0] == "--plan" {
		b, e := os.ReadFile(args[1])
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		var n networkconfig.NetworkPlan
		if e = json.Unmarshal(b, &n); e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		changes, e := networkconfig.PrepareNetworkConfiguration(args[2], &n)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		b, e = json.MarshalIndent(map[string]any{"network": n, "configuration": changes}, "", "  ")
		if e == nil {
			e = os.WriteFile(args[3], b, 0600)
		}
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		return 0
	}
	if len(args) == 3 && args[0] == "--capture-offline" {
		root, e := filepath.Abs(args[1])
		if e != nil {
			return 1
		}
		cfg, e := config.Load(filepath.Join(root, "MultiServer", "config.json"))
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		s := supervisor.New(cfg.Servers, supervisor.WindowsFactory{}, supervisor.Options{})
		s.DetectExternal(cfg.Servers)
		store := accountstore.New(filepath.Join(root, "MultiServer", "account-store", "accounts.json"), accountstore.DPAPIProtector{Entropy: []byte("NetherNetMultiServer/account-store/v1")})
		m := migration.NewManager(root, s, func() ([]byte, error) {
			a, e := store.Load()
			if e != nil {
				return nil, e
			}
			return migration.ExportAccounts(a)
		})
		id, e := m.Start(ctx, args[2])
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		last := ""
		for {
			j, e := m.Status(id)
			if e != nil {
				fmt.Fprintln(os.Stderr, e)
				return 1
			}
			if j.Phase != last {
				fmt.Println(j.Phase)
				last = j.Phase
			}
			if j.Done {
				if j.Error != "" {
					fmt.Fprintln(os.Stderr, j.Error)
					return 1
				}
				fmt.Println(j.Path)
				return 0
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	if len(args) == 2 && args[0] == "--verify" {
		m, e := migration.VerifyArchive(ctx, args[1])
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		fmt.Printf("Verified migration format %d, %d payload files\n", m.FormatVersion, len(m.Files))
		return 0
	}
	if len(args) == 7 && args[0] == "--restore-network" {
		r, e := migration.Restore(ctx, migration.RestoreOptions{ArchivePath: args[1], Destination: args[2], LANAddress: args[3], BroadcastAddress: args[4], ServerAddresses: map[string]string{"creative": args[5], "survival": args[6]}})

		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		fmt.Println("Restored; activation remains gated:", r.Root)
		return 0
	}
	if len(args) == 5 && args[0] == "--restore" {
		r, e := migration.Restore(ctx, migration.RestoreOptions{ArchivePath: args[1], Destination: args[2], LANAddress: args[3], BroadcastAddress: args[4]})
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		fmt.Printf("Restored %s; %d account(s) require sign-in. Run the setup wizard to configure and activate.\n", r.Root, r.AccountsRequiringSignIn)
		return 0
	}
	fmt.Fprintln(os.Stderr, "Usage: NetherNetMigrationArchive.exe [--verify snapshot.zip | --restore snapshot.zip NEW_FOLDER IPv4 BROADCAST]")
	return 2
}
