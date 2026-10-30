package contentpacks

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type PackType string

const (
	Behavior PackType = "behavior"
	Resource PackType = "resource"
)

type Dependency struct {
	UUID       string `json:"uuid,omitempty"`
	ModuleName string `json:"module_name,omitempty"`
	Version    [3]int `json:"version"`
}

func (d *Dependency) UnmarshalJSON(data []byte) error {
	var raw struct {
		UUID       string          `json:"uuid"`
		ModuleName string          `json:"module_name"`
		Version    json.RawMessage `json:"version"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	d.UUID, d.ModuleName = raw.UUID, raw.ModuleName
	if len(raw.Version) == 0 || string(raw.Version) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw.Version, &d.Version); err == nil {
		return nil
	}
	var version string
	if err := json.Unmarshal(raw.Version, &version); err != nil {
		return fmt.Errorf("dependency version must be a three-number array or dotted string")
	}
	parts := strings.Split(version, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return fmt.Errorf("invalid dependency version %q", version)
	}
	for i, part := range parts {
		if suffix := strings.IndexAny(part, "-+"); suffix >= 0 {
			part = part[:suffix]
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			// Marketplace-style packs sometimes use a channel label such as
			// "beta". Bedrock treats that as an unpinned module dependency.
			d.Version = [3]int{}
			return nil
		}
		d.Version[i] = value
	}
	return nil
}

type Pack struct {
	ImportSHA256                  string `json:"import_sha256,omitempty"`
	Name, Description, UUID, Root string
	Version, MinEngineVersion     [3]int
	Type                          PackType
	Dependencies                  []Dependency
	ArchiveData                   []byte `json:"-"`
	SourceArchive                 string `json:"-"`
}
type Import struct {
	Source string
	Packs  []Pack
}
type manifest struct {
	Header struct {
		Name, Description, UUID   string
		Version, MinEngineVersion [3]int `json:"-"`
	} `json:"header"`
	Modules []struct {
		Type, UUID string
		Version    [3]int
	} `json:"modules"`
	Dependencies []Dependency `json:"dependencies"`
}

func parseManifest(b []byte, root string) (Pack, error) {
	var raw struct {
		Header struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			UUID        string `json:"uuid"`
			Version     [3]int `json:"version"`
			Min         [3]int `json:"min_engine_version"`
		} `json:"header"`
		Modules []struct {
			Type string `json:"type"`
		} `json:"modules"`
		Dependencies []Dependency `json:"dependencies"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return Pack{}, fmt.Errorf("decode manifest: %w", err)
	}
	if raw.Header.Name == "" || raw.Header.UUID == "" {
		return Pack{}, fmt.Errorf("manifest header name and uuid are required")
	}
	if !validPackUUID(raw.Header.UUID) {
		return Pack{}, fmt.Errorf("manifest UUID must be a canonical UUID")
	}
	raw.Header.UUID = strings.ToLower(raw.Header.UUID)
	for _, v := range raw.Header.Version {
		if v < 0 {
			return Pack{}, fmt.Errorf("negative pack version")
		}
	}
	for i, d := range raw.Dependencies {
		if d.UUID != "" {
			if !validPackUUID(d.UUID) {
				return Pack{}, fmt.Errorf("dependency UUID must be canonical")
			}
			raw.Dependencies[i].UUID = strings.ToLower(d.UUID)
		}
	}
	p := Pack{Name: raw.Header.Name, Description: raw.Header.Description, UUID: raw.Header.UUID, Version: raw.Header.Version, MinEngineVersion: raw.Header.Min, Root: root, Dependencies: raw.Dependencies}
	for _, m := range raw.Modules {
		if m.Type == "data" || m.Type == "script" {
			p.Type = Behavior
		}
		if m.Type == "resources" {
			p.Type = Resource
		}
	}
	if p.Type == "" {
		return Pack{}, fmt.Errorf("manifest has no data or resources module")
	}
	return p, nil
}

var packUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validPackUUID(id string) bool { return packUUIDPattern.MatchString(id) }
