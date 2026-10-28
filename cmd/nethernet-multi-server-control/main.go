package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"nethernet-multi-server/internal/fleet"
	"nethernet-multi-server/internal/snapshotschedule"
	"os"
	"path/filepath"
	"time"

	"nethernet-multi-server/internal/adminapi"
	"nethernet-multi-server/internal/adminpipe"
	"nethernet-multi-server/internal/supervisor"
)

type operation struct{ method, serverID string }

func planLifecycle(action string, statuses []supervisor.Status) ([]operation, error) {
	var operations []operation
	switch action {
	case "start-all":
		for _, status := range statuses {
			if status.State == supervisor.Stopped {
				operations = append(operations, operation{"server.start", status.ID})
			}
		}
	case "stop-all":
		for _, status := range statuses {
			if status.State == supervisor.ExternallyRunning {
				return nil, fmt.Errorf("%s is externally running; stop it from its original console first", status.Name)
			}
		}
		for _, status := range statuses {
			if status.State == supervisor.Running {
				operations = append(operations, operation{"server.stop", status.ID})
			}
		}
		operations = append(operations, operation{"system.shutdown", ""})
	default:
		return nil, errors.New("usage: NetherNetMultiServerControl.exe start-all|stop-all")
	}
	return operations, nil
}

func call(ctx context.Context, method string, params any, target any) error {
	conn, err := adminpipe.DialCurrentUser(ctx, adminpipe.DefaultName)
	if err != nil {
		return err
	}
	defer conn.Close()
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	if err = adminapi.WriteRequest(conn, adminapi.Request{ID: "control", Method: method, Params: b}); err != nil {
		return err
	}
	response, err := adminapi.ReadResponse(conn)
	if err != nil {
		return err
	}
	if response.Error != nil {
		return errors.New(response.Error.Message)
	}
	if target != nil {
		b, _ = json.Marshal(response.Result)
		return json.Unmarshal(b, target)
	}
	return nil
}

func run(args []string) error {
	if len(args) == 1 && args[0] == "snapshot-scheduled" {
		return scheduledSnapshot()
	}
	if len(args) != 1 || (args[0] != "start-all" && args[0] != "stop-all" && args[0] != "restart-all") {
		return errors.New("usage: NetherNetMultiServerControl.exe start-all|stop-all|restart-all")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	return fleet.NativeEngine(filepath.Dir(exe)).Run(ctx, args[0], func(message string) { fmt.Println(message) })
}

func scheduledSnapshot() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	directory := filepath.Dir(exe)
	settings, err := snapshotschedule.Load(directory)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return nil
	}
	log, err := os.OpenFile(filepath.Join(directory, "snapshot-schedule.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	notify := func(message string) { fmt.Fprintf(log, "%s %s\n", time.Now().Format(time.RFC3339), message) }
	ctx, cancel := context.WithTimeout(context.Background(), 11*time.Hour)
	defer cancel()
	client := fleet.PipeClient{}
	host := &fleet.NativeHost{Directory: directory, Client: client}
	notify("Starting scheduled full snapshot")
	if err = host.Ensure(ctx); err == nil {
		err = snapshotschedule.Capture(ctx, client, settings.OutputRoot, time.Now(), notify)
	}
	if err != nil {
		notify("Failed: " + err.Error())
	}
	return err
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	fmt.Println("Completed successfully.")
}
