package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"nethernet-multi-server/internal/adminapi"
	"nethernet-multi-server/internal/adminpipe"
	"os"
	"time"
)

func call(ctx context.Context, method string, params any) error {
	conn, err := adminpipe.DialCurrentUser(ctx, adminpipe.DefaultName)
	if err != nil {
		return err
	}
	defer conn.Close()
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	if err = adminapi.WriteRequest(conn, adminapi.Request{ID: "packctl", Method: method, Params: b}); err != nil {
		return err
	}
	response, err := adminapi.ReadResponse(conn)
	if err != nil {
		return err
	}
	if response.Error != nil {
		return errors.New(response.Error.Message)
	}
	return nil
}

func main() {
	if len(os.Args) == 4 && os.Args[1] == "role" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := call(ctx, "player.role.set", struct {
			XUID string `json:"xuid"`
			Role string `json:"role"`
		}{os.Args[2], os.Args[3]}); err != nil {
			fmt.Fprintln(os.Stderr, "player.role.set:", err)
			os.Exit(1)
		}
		fmt.Println("Player role updated on all servers.")
		return
	}
	if len(os.Args) < 4 || os.Args[1] != "disable" {
		fmt.Fprintln(os.Stderr, "usage: nethernet-packctl disable SERVER UUID [UUID...] | role XUID ROLE")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	server, uuids := os.Args[2], os.Args[3:]
	steps := []struct {
		method string
		params any
	}{
		{"server.stop", map[string]string{"server_id": server}},
		{"pack.stage_many", struct {
			ServerID  string   `json:"server_id"`
			PackUUIDs []string `json:"pack_uuids"`
			Enable    bool     `json:"enable"`
		}{server, uuids, false}},
		{"packs.apply", map[string]string{"server_id": server}},
		{"server.start", map[string]string{"server_id": server}},
	}
	for _, step := range steps {
		if err := call(ctx, step.method, step.params); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", step.method, err)
			os.Exit(1)
		}
	}
	fmt.Printf("Disabled %d selected pack(s) and linked dependencies on %s.\n", len(uuids), server)
}
