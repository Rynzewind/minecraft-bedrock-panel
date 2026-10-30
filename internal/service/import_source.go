package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"nethernet-multi-server/internal/addonupdates"
	"nethernet-multi-server/internal/contentpacks"
	"os"
	"path/filepath"
	"time"
)

func (c *Controller) identifyImportedSource(ctx context.Context, path string) (addonupdates.ImportSourceResult, error) {
	result := addonupdates.ImportSourceResult{}
	if c.packs == nil || c.addonSources == nil || c.addonCredentials == nil {
		return result, fmt.Errorf("source identification is unavailable")
	}
	key, err := c.addonCredentials.Load()
	if err != nil {
		result.Message = "Imported successfully. Save a CurseForge API key in Updates settings to identify download sources."
		return result, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return result, err
	}
	data, err := io.ReadAll(io.LimitReader(f, contentpacks.DefaultLimits.MaxCompressedBytes+1))
	f.Close()
	if err != nil || int64(len(data)) > contentpacks.DefaultLimits.MaxCompressedBytes {
		return result, fmt.Errorf("source archive exceeds identification limits or could not be read")
	}
	in, err := contentpacks.Inspect(path, contentpacks.DefaultLimits)
	if err != nil {
		return result, err
	}
	hash := sha256.Sum256(data)
	expected := hex.EncodeToString(hash[:])
	for _, pack := range in.Packs {
		if pack.ImportSHA256 != expected {
			return result, fmt.Errorf("download changed during source lookup; import it again")
		}
	}
	provider, ok := c.addonProvider(key).(interface {
		IdentifyDownload(context.Context, []byte, int64) (addonupdates.Project, addonupdates.ReleaseFile, error)
	})
	if !ok {
		return result, fmt.Errorf("exact source identification is unavailable")
	}
	lookup, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	project, file, err := provider.IdentifyDownload(lookup, data, addonupdates.DownloadFileID(path))
	if err != nil {
		if search, ok := c.addonProvider(key).(interface {
			IdentifyByNames(context.Context, []byte, []string) (addonupdates.Project, addonupdates.ReleaseFile, error)
		}); ok {
			names := []string{}
			for _, pack := range in.Packs {
				names = append(names, pack.Name)
			}
			names = append(names, filepath.Base(path))
			project, file, err = search.IdentifyByNames(lookup, data, names)
		}
	}
	if err != nil {
		result.Message = "Imported successfully. Source not linked: " + err.Error()
		return result, nil
	}
	result.Found = true
	result.ProjectID = project.ID
	result.URL = project.Website()
	result.ProjectName = project.Name
	if result.URL == "" {
		return result, fmt.Errorf("matched project has no verified Bedrock URL")
	}
	records, err := c.addonSources.ListRecords()
	if err != nil {
		return result, err
	}
	inventory := c.packs.InventoryState()
	for _, incoming := range in.Packs {
		eligible := false
		for _, stored := range inventory {
			if stored.UUID == incoming.UUID && stored.Version == incoming.Version && stored.ImportSHA256 == expected {
				eligible = true
			}
		}
		source, exists := records[incoming.UUID]
		if !eligible || exists && (source.LocallyModified || source.Provider != "" || source.URL != "" || source.ProjectID != 0) {
			result.Preserved++
			continue
		}
		source = addonupdates.Source{Provider: "curseforge", ProjectID: project.ID, URL: result.URL, Channel: "stable", LastFileID: file.ID, Notes: fmt.Sprintf("Automatically identified from exact imported archive checksum; CurseForge file %d. Latest release check pending.", file.ID)}
		if err = c.addonSources.SaveRecord(incoming.UUID, source); err != nil {
			return result, err
		}
		result.Linked++
	}
	result.Message = fmt.Sprintf("Source identified: %s. Linked %d pack(s); preserved %d existing or unmatched library entry/entries.", project.Name, result.Linked, result.Preserved)
	return result, nil
}
