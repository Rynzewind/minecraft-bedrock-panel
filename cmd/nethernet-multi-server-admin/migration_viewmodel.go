package main

import (
	"fmt"
	"nethernet-multi-server/internal/migration"
)

func snapshotSummary(s migration.JobStatus) string {
	text := fmt.Sprintf("%s — %d%%", s.Phase, s.Progress)
	if s.Phase == "warning" {
		text = fmt.Sprintf("Scheduled backup: warning players; restart in %d minute(s). The backup will run even with players online.", s.WarningSeconds/60)
	}
	if s.Done && s.Validated {
		text = "Verified migration snapshot ready:\r\n" + s.Path + "\r\nCopy this ZIP and NetherNetSetup.exe to the new PC. Run the installer, choose Restore saved servers/worlds/addons, and select this ZIP."
	}
	if s.Error != "" {
		text += "\r\nSnapshot error: " + s.Error
	}
	if s.RestartError != "" {
		text += "\r\nServer restart needs attention: " + s.RestartError
	}
	if s.Warning != "" {
		text += "\r\nNeeds attention: " + s.Warning
	}
	return text
}
