package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"nethernet-multi-server/internal/accounts"
	"nethernet-multi-server/internal/accountstore"
	"nethernet-multi-server/internal/addonupdates"
	"nethernet-multi-server/internal/adminapi"
	"nethernet-multi-server/internal/contentpacks"
	"nethernet-multi-server/internal/serversettings"
	"nethernet-multi-server/internal/sessionhistory"
	"nethernet-multi-server/internal/supervisor"
	"strings"
	"sync"
)

type PublicAccount struct {
	ID, PersonLabel, EmailLabel, Gamertag, XUID, State string
	Order                                              int
	Healthy                                            bool
}
type Status struct {
	Accounts []PublicAccount              `json:"accounts"`
	Players  []accountstore.PlayerMapping `json:"players"`
	Active   []accounts.ActiveLease       `json:"active"`
	Roles    map[string]string            `json:"roles,omitempty"`
}
type pendingEnrollment struct {
	enrollment *accounts.Enrollment
	accountID  string
}
type Controller struct {
	addonProviderFactory          func(string) addonProvider
	addonOrigins                  map[string]addonOrigin
	addonSources                  *addonupdates.Store
	addonConfigPath, addonStaging string
	addonCredentials              *addonupdates.Credentials
	addonResults                  map[string]addonCheckResult
	updates                       updateManager
	operationMu                   sync.Mutex
	shuttingDown                  bool
	migration                     migrationJobs
	mu                            sync.Mutex
	store                         *accountstore.Store
	manager                       *accounts.Manager
	auth                          accounts.Authenticator
	enrollments                   map[string]pendingEnrollment
	next                          int
	supervisor                    serverSupervisor
	packs                         packManager
	shutdown                      func()
	settings                      settingsManager
	history                       sessionHistory
	permissions                   playerPermissions
}
type sessionHistory interface {
	List(int) []sessionhistory.Event
}
type playerPermissions interface {
	SetAll(string, string) error
	Roles() (map[string]string, error)
}
type serverSupervisor interface {
	Statuses() []supervisor.Status
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Restart(context.Context, string) error
	RecentLog(string) string
}
type packManager interface {
	InventoryState() []contentpacks.InventoryItem
	ImportArchive(string) ([]contentpacks.StoredPack, error)
	Stage(string, string, bool) error
	StageMany(string, []string, bool) error
	Pending(string) []contentpacks.Operation
	Apply(string) error
	StageResourcePolicy(string, bool) error
	Uninstall(string) error
	UninstallPlan([]string) ([]contentpacks.StoredPack, error)
	UninstallMany([]string) error
	CancelPending(string) error
}
type settingsManager interface {
	Get(string) (serversettings.Settings, error)
	Save(string, serversettings.Settings) error
	Pending(string) bool
	ClearPending(string) error
}

func NewController(store *accountstore.Store, m *accounts.Manager, a accounts.Authenticator) *Controller {
	return &Controller{store: store, manager: m, auth: a, enrollments: map[string]pendingEnrollment{}}
}
func NewManagedController(store *accountstore.Store, m *accounts.Manager, a accounts.Authenticator, s serverSupervisor, p packManager) *Controller {
	c := NewController(store, m, a)
	c.supervisor = s
	c.packs = p
	return c
}
func (c *Controller) SetShutdown(shutdown func())                        { c.shutdown = shutdown }
func (c *Controller) SetServerSettings(settings settingsManager)         { c.settings = settings }
func (c *Controller) SetSessionHistory(history sessionHistory)           { c.history = history }
func (c *Controller) SetPlayerPermissions(permissions playerPermissions) { c.permissions = permissions }

type coded struct{ code, msg string }

func (e coded) Error() string { return e.msg }
func (e coded) Code() string  { return e.code }
func (c *Controller) Handle(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	if !migrationReadMethod(method) {
		c.operationMu.Lock()
		defer c.operationMu.Unlock()
		if c.shuttingDown {
			return nil, coded{"maintenance", "The helper is shutting down for maintenance."}
		}
		if c.updates != nil && (c.updates.Busy() || c.updates.Status().Recovery) && method != "updates.recover" && !(method == "server.stop" && !c.updates.Busy()) {
			return nil, coded{"update_busy", "Server update or recovery is pending. Use the Updates tab."}
		}
		if c.migration != nil && c.migration.Busy() {
			return nil, coded{"migration_busy", "A snapshot is in progress. Wait for it to finish before making changes."}
		}
	}
	if strings.HasPrefix(method, "addons.") {
		return c.handleAddonUpdates(ctx, method, raw)
	}
	if strings.HasPrefix(method, "updates.") {
		return c.handleUpdates(ctx, method, raw)
	}
	if strings.HasPrefix(method, "migration.") {
		return c.handleMigration(ctx, method, raw)
	}
	switch method {
	case "player.role.set":
		var p struct {
			XUID string `json:"xuid"`
			Role string `json:"role"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.permissions == nil {
			return nil, coded{"not_found", "player permissions are not configured"}
		}
		return nil, c.permissions.SetAll(p.XUID, p.Role)
	case "sessions.list":
		var p struct {
			Limit int `json:"limit"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.history == nil {
			return []sessionhistory.Event{}, nil
		}
		if p.Limit <= 0 || p.Limit > 5000 {
			p.Limit = 1000
		}
		return c.history.List(p.Limit), nil
	case "server.settings.get":
		var p struct {
			ServerID string `json:"server_id"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.settings == nil {
			return nil, coded{"not_found", "server settings are not configured"}
		}
		return c.settings.Get(p.ServerID)
	case "server.settings.save":
		var p struct {
			ServerID string                  `json:"server_id"`
			Settings serversettings.Settings `json:"settings"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.settings == nil {
			return nil, coded{"not_found", "server settings are not configured"}
		}
		return nil, c.settings.Save(p.ServerID, p.Settings)
	case "system.shutdown":
		if c.supervisor == nil || c.shutdown == nil {
			return nil, coded{"not_found", "managed shutdown is not configured"}
		}
		for _, status := range c.supervisor.Statuses() {
			if status.State == supervisor.ExternallyRunning {
				return nil, coded{"invalid_server_state", fmt.Sprintf("%s is externally running and cannot be stopped safely", status.Name)}
			}
		}
		for _, status := range c.supervisor.Statuses() {
			if status.State == supervisor.Running {
				if err := c.supervisor.Stop(ctx, status.ID); err != nil {
					return nil, err
				}
			}
		}
		c.shuttingDown = true
		c.shutdown()
		return map[string]bool{"stopped": true}, nil
	case "servers.list":
		if c.supervisor == nil {
			return []supervisor.Status{}, nil
		}
		statuses := c.supervisor.Statuses()
		for i := range statuses {
			if c.packs != nil {
				statuses[i].Pending = len(c.packs.Pending(statuses[i].ID)) > 0
			}
			if c.settings != nil && c.settings.Pending(statuses[i].ID) {
				statuses[i].Pending = true
			}
			if c.manager != nil {
				for _, active := range c.manager.Snapshot().Active {
					if active.Server == statuses[i].Name {
						statuses[i].Players++
					}
				}
			}
		}
		return statuses, nil
	case "server.start", "server.stop", "server.restart":
		var p struct {
			ServerID string `json:"server_id"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.supervisor == nil {
			return nil, coded{"not_found", "server controls are not configured"}
		}
		var err error
		if method == "server.start" {
			err = c.supervisor.Start(ctx, p.ServerID)
		} else if method == "server.stop" {
			err = c.supervisor.Stop(ctx, p.ServerID)
		} else {
			err = c.supervisor.Restart(ctx, p.ServerID)
		}
		return nil, err
	case "server.progress":
		var p struct {
			ServerID string `json:"server_id"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.supervisor != nil {
			for _, s := range c.supervisor.Statuses() {
				if s.ID == p.ServerID {
					return supervisor.Progress{Status: s, Log: c.supervisor.RecentLog(s.ID)}, nil
				}
			}
		}
		return nil, coded{"not_found", "server not found"}
	case "server.logs":
		var p struct {
			ServerID string `json:"server_id"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.supervisor == nil {
			return "", nil
		}
		return c.supervisor.RecentLog(p.ServerID), nil
	case "packs.list":
		if c.packs == nil {
			return []contentpacks.InventoryItem{}, nil
		}
		return c.packs.InventoryState(), nil
	case "pack.import":
		var p struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.packs == nil {
			return nil, coded{"not_found", "pack controls are not configured"}
		}
		return c.packs.ImportArchive(p.Path)
	case "pack.stage":
		var p struct {
			ServerID string `json:"server_id"`
			PackUUID string `json:"pack_uuid"`
			Enable   bool   `json:"enable"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.packs == nil {
			return nil, coded{"not_found", "pack controls are not configured"}
		}
		return nil, c.packs.Stage(p.ServerID, p.PackUUID, p.Enable)
	case "pack.stage_many":
		var p struct {
			ServerID  string   `json:"server_id"`
			PackUUIDs []string `json:"pack_uuids"`
			Enable    bool     `json:"enable"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.packs == nil {
			return nil, coded{"not_found", "pack controls are not configured"}
		}
		return nil, c.packs.StageMany(p.ServerID, p.PackUUIDs, p.Enable)
	case "pack.policy":
		var p struct {
			ServerID string `json:"server_id"`
			Required bool   `json:"required"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.packs == nil {
			return nil, coded{"not_found", "pack controls are not configured"}
		}
		return nil, c.packs.StageResourcePolicy(p.ServerID, p.Required)
	case "pack.uninstall":
		var p struct {
			PackUUID string `json:"pack_uuid"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.packs == nil {
			return nil, coded{"not_found", "pack controls are not configured"}
		}
		return nil, c.packs.Uninstall(p.PackUUID)
	case "pack.uninstall.preview", "pack.uninstall_many":
		var p struct {
			PackUUIDs []string `json:"pack_uuids"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.packs == nil {
			return nil, coded{"not_found", "pack controls are not configured"}
		}
		if method == "pack.uninstall.preview" {
			return c.packs.UninstallPlan(p.PackUUIDs)
		}
		return nil, c.packs.UninstallMany(p.PackUUIDs)
	case "packs.pending":
		var p struct {
			ServerID string `json:"server_id"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.packs == nil {
			return []contentpacks.Operation{}, nil
		}
		return c.packs.Pending(p.ServerID), nil
	case "packs.cancel":
		var p struct {
			ServerID string `json:"server_id"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.packs == nil {
			return nil, coded{"not_found", "pack controls are not configured"}
		}
		return nil, c.packs.CancelPending(p.ServerID)
	case "packs.apply":
		var p struct {
			ServerID string `json:"server_id"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		if c.packs == nil {
			return nil, coded{"not_found", "pack controls are not configured"}
		}
		if c.supervisor != nil {
			for _, s := range c.supervisor.Statuses() {
				if s.ID == p.ServerID && s.State != supervisor.Stopped {
					return nil, coded{"invalid_server_state", "pack changes can only be applied while the server is stopped"}
				}
			}
		}
		return nil, c.packs.Apply(p.ServerID)
	case "status.list":
		return c.status(), nil
	case "account.enroll.begin":
		var p adminapi.EnrollmentParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		code, enrollment, err := accounts.BeginEnrollment(ctx, c.store, c.auth, accounts.Labels{Person: p.PersonLabel, Email: p.EmailLabel})
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.next++
		id := fmt.Sprint(c.next)
		c.enrollments[id] = pendingEnrollment{enrollment: enrollment}
		c.mu.Unlock()
		return map[string]any{"enrollment_id": id, "user_code": code.UserCode, "verification_uri": code.VerificationURI, "verification_uri_complete": code.VerificationURIComplete, "expiry": code.Expiry}, nil
	case "account.enroll.complete":
		var p adminapi.EnrollmentCompleteParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		c.mu.Lock()
		pending, ok := c.enrollments[p.EnrollmentID]
		delete(c.enrollments, p.EnrollmentID)
		c.mu.Unlock()
		if !ok {
			return nil, coded{"not_found", "enrollment not found"}
		}
		var a accountstore.Account
		var err error
		if pending.accountID == "" {
			a, err = pending.enrollment.Complete(ctx)
		} else {
			a, err = pending.enrollment.CompleteReplacement(ctx, pending.accountID)
		}
		if err != nil {
			return nil, err
		}
		if err = c.manager.Reload(); err != nil {
			return nil, err
		}
		a.EncryptedToken = ""
		return a, nil
	case "account.reauthenticate":
		var p struct {
			AccountID string `json:"account_id"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		var selected *accountstore.Account
		for _, item := range c.manager.Snapshot().Accounts {
			if item.ID == p.AccountID {
				copy := item
				selected = &copy
				break
			}
		}
		if selected == nil {
			return nil, coded{"not_found", "account not found"}
		}
		code, enrollment, err := accounts.BeginEnrollment(ctx, c.store, c.auth, accounts.Labels{Person: selected.PersonLabel, Email: selected.EmailLabel})
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.next++
		id := fmt.Sprint(c.next)
		c.enrollments[id] = pendingEnrollment{enrollment: enrollment, accountID: p.AccountID}
		c.mu.Unlock()
		return map[string]any{"enrollment_id": id, "user_code": code.UserCode, "verification_uri": code.VerificationURI, "verification_uri_complete": code.VerificationURIComplete, "expiry": code.Expiry}, nil
	case "account.labels.update":
		var p adminapi.AccountLabelsParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		return nil, c.manager.UpdateLabels(p.AccountID, p.PersonLabel, p.EmailLabel)
	case "account.remove":
		var p struct {
			AccountID string `json:"account_id"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		err := c.manager.RemoveAccount(p.AccountID)
		return nil, mapManagerError(err)
	case "account.reorder":
		var p struct {
			AccountIDs []string `json:"account_ids"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		return nil, c.manager.Reorder(p.AccountIDs)
	case "player.reassign.preview":
		var p adminapi.ReassignParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		v, err := c.manager.PreviewReassign(p.ConsoleXUID, p.AccountID)
		return v, mapManagerError(err)
	case "player.reassign.commit":
		var p adminapi.ReassignParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		return nil, mapManagerError(c.manager.Reassign(p.ConsoleXUID, p.AccountID))
	case "player.unpair":
		var p adminapi.PlayerParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, coded{"invalid_request", err.Error()}
		}
		return nil, mapManagerError(c.manager.Unpair(p.ConsoleXUID))
	default:
		return nil, coded{"not_found", "unknown method"}
	}
}
func mapManagerError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, accounts.ErrPlayerActive):
		return coded{"player_active", err.Error()}
	case errors.Is(err, accounts.ErrAccountAssigned):
		return coded{"account_assigned", err.Error()}
	case errors.Is(err, accounts.ErrPoolExhausted):
		return coded{"pool_exhausted", err.Error()}
	default:
		return err
	}
}
func (c *Controller) status() Status {
	s := c.manager.Snapshot()
	out := Status{Players: s.Players, Active: s.Active}
	if c.permissions != nil {
		if roles, err := c.permissions.Roles(); err == nil {
			out.Roles = roles
		}
	}
	busy := map[string]bool{}
	for _, a := range s.Active {
		busy[a.AccountID] = true
	}
	for _, a := range s.Accounts {
		state := "Available"
		if busy[a.ID] {
			state = "In use"
		} else if !a.Healthy {
			state = "Needs sign-in"
		}
		out.Accounts = append(out.Accounts, PublicAccount{ID: a.ID, PersonLabel: a.PersonLabel, EmailLabel: a.EmailLabel, Gamertag: a.Gamertag, XUID: a.XUID, State: state, Order: a.Order, Healthy: a.Healthy})
	}
	return out
}
