package service

import (
	"context"
	"encoding/json"
	"nethernet-multi-server/internal/adminapi"
	"nethernet-multi-server/internal/migration"
)

type migrationJobs interface {
	Busy() bool
	Start(context.Context, string) (string, error)
	Status(string) (migration.JobStatus, error)
	Cancel(string) error
}

func (c *Controller) SetMigration(m migrationJobs) { c.migration = m }
func migrationReadMethod(method string) bool {
	switch method {
	case "account.capabilities", "online.status", "online.settings.get", "online.players.list", "online.logs", "online.diagnostics", "online.players.resolve", "addons.credentials.status", "addons.sources.records", "addons.check", "addons.download", "addons.credentials.test", "addons.sources.list", "updates.check", "updates.status", "status.list", "servers.list", "server.progress", "server.logs", "server.settings.get", "packs.list", "packs.pending", "pack.uninstall.preview", "player.reassign.preview", "sessions.list", "migration.status", "migration.cancel":
		return true
	}
	return false
}
func (c *Controller) handleMigration(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	if c.migration == nil {
		return nil, coded{"not_found", "Snapshot support is not configured."}
	}
	switch method {
	case "migration.start":
		var p adminapi.MigrationStartParams
		if e := json.Unmarshal(raw, &p); e != nil {
			return nil, coded{"invalid_request", "Invalid snapshot request."}
		}
		var id string
		var e error
		if p.IncludeAddons != nil || p.Kind != "" {
			withOptions, ok := c.migration.(interface {
				StartWithOptions(context.Context, string, migration.SnapshotOptions) (string, error)
			})
			if !ok {
				return nil, coded{"unsupported", "This helper does not support addon snapshot options."}
			}
			include := true
			if p.IncludeAddons != nil {
				include = *p.IncludeAddons
			}
			id, e = withOptions.StartWithOptions(ctx, p.OutputDirectory, migration.SnapshotOptions{IncludeAddons: include, Kind: p.Kind})
		} else {
			id, e = c.migration.Start(ctx, p.OutputDirectory)
		}
		if e != nil {
			return nil, e
		}
		return map[string]string{"job_id": id}, nil
	case "migration.status":
		var p adminapi.MigrationJobParams
		if e := json.Unmarshal(raw, &p); e != nil {
			return nil, e
		}
		return c.migration.Status(p.JobID)
	case "migration.cancel":
		var p adminapi.MigrationJobParams
		if e := json.Unmarshal(raw, &p); e != nil {
			return nil, e
		}
		return nil, c.migration.Cancel(p.JobID)
	default:
		return nil, coded{"not_found", "Unknown snapshot operation."}
	}
}
