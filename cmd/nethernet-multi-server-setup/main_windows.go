package main

import (
	"context"

	"fmt"

	"nethernet-multi-server/internal/accountstore"
	"nethernet-multi-server/internal/config"
	"nethernet-multi-server/internal/migration"
	"nethernet-multi-server/internal/setup"

	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if len(os.Args) > 1 {
		os.Exit(command(os.Args[1:]))
	}
	runSetup()
}
func command(args []string) int {
	ctx := context.Background()
	if len(args) == 2 && args[0] == "--repair" {
		runRepair(args[1])
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
		if e == nil {
			e = setup.InstallTools(r.Root)
		}
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		fmt.Println("Restored and updated; activation remains gated:", r.Root)
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
	fmt.Fprintln(os.Stderr, "Usage: NetherNetMultiServerSetup.exe [--verify snapshot.zip | --restore snapshot.zip NEW_FOLDER IPv4 BROADCAST]")
	return 2
}
