package main

import "strings"

type minecraftStep struct {
	Text  string
	State int
}

func minecraftProgressSteps(method, message string) []minecraftStep {
	if method == "settings.save" {
		return []minecraftStep{{Text: "Save server settings", State: 1}}
	}
	labels := []string{}
	if method == "server.stop" || method == "server.restart" {
		labels = append(labels, "Save world and stop Minecraft")
	}
	if method != "server.stop" {
		labels = append(labels, "Apply pending addon/settings changes")
	}
	if method == "server.start" || method == "server.restart" {
		labels = append(labels, "Launch Minecraft", "Check connection readiness")
	}
	active := -1
	switch {
	case strings.Contains(message, "Saving world"):
		active = 0
	case strings.Contains(message, "Applying"):
		active = 0
		if method == "server.restart" {
			active = 1
		}
	case strings.Contains(message, "Launching"):
		active = len(labels) - 2
	case strings.Contains(message, "Checking"):
		active = len(labels) - 1
	}
	out := make([]minecraftStep, len(labels))
	for i, label := range labels {
		state := 0
		if active >= 0 {
			if i < active {
				state = 2
			} else if i == active {
				state = 1
			}
		}
		out[i] = minecraftStep{label, state}
	}
	return out
}
