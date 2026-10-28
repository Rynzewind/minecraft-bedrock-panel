package service

import (
	"context"
	"encoding/json"
	"fmt"
	"nethernet-multi-server/internal/addonupdates"
	"nethernet-multi-server/internal/config"
	"nethernet-multi-server/internal/contentpacks"
	"os"
	"regexp"
	"strconv"
	"time"
)

type addonCheckResult struct {
	UUID    string
	Project addonupdates.Project
	Files   []addonupdates.ReleaseFile
	Checked time.Time
	Source  addonupdates.Source
}
type updatePacks interface {
	PreviewUpdate(string, string, []string, map[string][3]int) (contentpacks.UpdatePreview, error)
	StageUpdateForEngines(string, map[string][3]int) error
}

func (c *Controller) SetAddonUpdates(credentials *addonupdates.Credentials, configPath, staging string) {
	c.addonCredentials = credentials
	c.addonConfigPath = configPath
	c.addonStaging = staging
	c.addonResults = map[string]addonCheckResult{}
}
func (c *Controller) validAddon(id string) bool {
	if c.packs == nil {
		return false
	}
	for _, p := range c.packs.InventoryState() {
		if p.UUID == id {
			return true
		}
	}
	return false
}
func (c *Controller) handleAddonUpdates(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	if method == "addons.sources.list" || method == "addons.sources.set" {
		return c.handleAddonSources(method, raw)
	}
	if c.addonSources == nil || c.addonCredentials == nil {
		return nil, fmt.Errorf("add-on updater requires the updated helper")
	}
	var req struct {
		UUID, Key, Path, PreviewID string
		Source                     addonupdates.Source
		Servers                    []string
		FileID                     int64
		Notes                      string
	}
	if len(raw) > 0 {
		if e := json.Unmarshal(raw, &req); e != nil {
			return nil, fmt.Errorf("invalid add-on update request")
		}
	}
	switch method {
	case "addons.credentials.status":
		return c.addonCredentials.Status()
	case "addons.credentials.save":
		return nil, c.addonCredentials.Set(req.Key)
	case "addons.credentials.remove":
		return nil, c.addonCredentials.Remove()
	case "addons.credentials.test":
		key, e := c.addonCredentials.Load()
		if e != nil {
			return nil, e
		}
		e = addonupdates.NewProvider(key).Test(ctx)
		if current, loadErr := c.addonCredentials.Load(); loadErr == nil && current == key {
			if saveErr := c.addonCredentials.MarkTest(e == nil); saveErr != nil {
				return nil, saveErr
			}
		}
		if e != nil {
			return nil, e
		}
		return c.addonCredentials.Status()
	case "addons.sources.records":
		return c.addonSources.ListRecords()
	}
	if !c.validAddon(req.UUID) && method != "addons.stage" {
		return nil, fmt.Errorf("select an installed pack")
	}
	switch method {
	case "addons.source.save":
		return nil, c.addonSources.SaveRecord(req.UUID, req.Source)
	case "addons.manual.checked":
		return nil, c.addonSources.MarkChecked(req.UUID, "manually reviewed; update status unverified", req.Notes)
	case "addons.check":
		rows, e := c.addonSources.ListRecords()
		if e != nil {
			return nil, e
		}
		src := rows[req.UUID]
		key, e := c.addonCredentials.Load()
		if e != nil {
			return nil, e
		}
		project, files, e := addonupdates.NewProvider(key).Check(ctx, src)
		state := "release list fetched; preview an archive to verify pack versions"
		if e != nil {
			state = e.Error()
		}
		if markErr := c.addonSources.MarkResult(req.UUID, src, state); markErr != nil {
			return nil, markErr
		}
		if e != nil {
			return nil, e
		}
		result := addonCheckResult{req.UUID, project, files, time.Now(), src}
		c.mu.Lock()
		c.addonResults[req.UUID] = result
		c.mu.Unlock()
		return result, nil
	case "addons.download":
		c.mu.Lock()
		result, ok := c.addonResults[req.UUID]
		c.mu.Unlock()
		if !ok || time.Since(result.Checked) > 30*time.Minute {
			return nil, fmt.Errorf("check this source again before downloading")
		}
		rows, e := c.addonSources.ListRecords()
		if e != nil {
			return nil, e
		}
		src := rows[req.UUID]
		if src.URL != result.Source.URL || src.ProjectID != result.Source.ProjectID || src.Channel != result.Source.Channel || src.Provider != result.Source.Provider {
			return nil, fmt.Errorf("source changed; check again")
		}
		var file addonupdates.ReleaseFile
		for _, f := range result.Files {
			if f.ID == req.FileID {
				file = f
				break
			}
		}
		if file.ID == 0 {
			return nil, fmt.Errorf("select a checked release")
		}
		key, e := c.addonCredentials.Load()
		if e != nil {
			return nil, e
		}
		path, e := addonupdates.NewProvider(key).Download(ctx, result.Project, file, c.addonStaging)
		if e != nil {
			return nil, e
		}
		defer os.Remove(path)
		req.Path = path
		b, _ := json.Marshal(req)
		return c.Handle(ctx, "addons.preview", b)
	case "addons.preview":
		manager, ok := c.packs.(updatePacks)
		if !ok {
			return nil, fmt.Errorf("pack update previews unavailable")
		}
		engines, e := c.addonEngines()
		if e != nil {
			return nil, e
		}
		return manager.PreviewUpdate(req.Path, req.UUID, req.Servers, engines)
	case "addons.stage":
		manager, ok := c.packs.(updatePacks)
		if !ok {
			return nil, fmt.Errorf("pack update staging unavailable")
		}
		engines, e := c.addonEngines()
		if e != nil {
			return nil, e
		}
		return nil, manager.StageUpdateForEngines(req.PreviewID, engines)
	}
	return nil, fmt.Errorf("unknown add-on update operation")
}

func (c *Controller) addonEngines() (map[string][3]int, error) {
	cfg, e := config.Load(c.addonConfigPath)
	if e != nil {
		return nil, fmt.Errorf("cannot read server engine versions")
	}
	engines := map[string][3]int{}
	re := regexp.MustCompile(`(?:Version:\s*)?(\d+)\.(\d+)\.(\d+)(?:\.\d+)?`)
	for _, s := range cfg.Servers {
		version := s.Version
		if c.supervisor != nil {
			if matches := regexp.MustCompile(`Version: (\d+\.\d+\.\d+(?:\.\d+)?)`).FindStringSubmatch(c.supervisor.RecentLog(s.ID)); len(matches) > 1 {
				version = matches[1]
			}
		}
		if matches := re.FindStringSubmatch(version); len(matches) > 3 {
			var v [3]int
			for i := 0; i < 3; i++ {
				v[i], _ = strconv.Atoi(matches[i+1])
			}
			engines[s.ID] = v
		}
	}

	return engines, nil
}
