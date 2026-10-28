package main

import (
	"fmt"
	"nethernet-multi-server/internal/supervisor"
	"strings"
	"time"
)

func serverProgressText(method string, s supervisor.Status, elapsed time.Duration, ready bool) string {
	stage := "Waiting for server command"
	if method == "packs.apply" {
		stage = "Applying addon changes and creating backups"
	} else if method == "settings.save" {
		stage = "Saving server settings"
	} else {
		switch s.Phase {
		case "applying_changes":
			stage = "Applying addon/settings changes and creating backups"
		case "launching":
			stage = "Launching Minecraft"
		case "stopping":
			stage = "Saving world and waiting for Minecraft to stop"
		case "failed":
			stage = "Minecraft reported a failure"
		case "running":
			stage = "Checking Minecraft connection readiness"
		}
	}
	if ready {
		stage = "Completed"
	}
	text := fmt.Sprintf("%s - %s elapsed", stage, elapsed.Truncate(time.Second))
	if elapsed >= 30*time.Second && !ready {
		text += "\r\nStill working. Large addons and world saves can take several minutes."
	}
	return text
}

func lastProgressLogLine(log string) string {
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) == 0 {
		return ""
	}
	line := strings.TrimSpace(lines[len(lines)-1])
	if len(line) > 1500 {
		line = line[len(line)-1500:]
	}
	return strings.ToValidUTF8(line, "?")
}

func serverOperationComplete(method string, s supervisor.Status, ready bool) bool {
	if method == "packs.apply" || method == "settings.save" {
		return true
	}
	if method == "server.stop" {
		return s.State == supervisor.Stopped
	}
	return s.State == supervisor.Running && s.PID > 0 && ready
}

func batchServerRows(rows []ServerRow, method string) (eligible []ServerRow, skipped []string) {
	for _, row := range rows {
		ok := (method == "server.start" && row.CanStart) || (method == "server.stop" && row.CanStop) || (method == "server.restart" && row.CanRestart)
		if ok {
			eligible = append(eligible, row)
		} else {
			skipped = append(skipped, row.Name+" ("+row.State+")")
		}
	}
	return
}
