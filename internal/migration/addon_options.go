package migration

import "strings"

// IncludeAddons controls the optional complete library and original downloads.
// Server-installed packs and world pack selections always travel with worlds.
type SnapshotOptions struct {
	IncludeAddons bool
	Kind          string
}

func filterAddonLibrary(files []FileEntry, include bool) []FileEntry {
	if include {
		return files
	}
	result := make([]FileEntry, 0, len(files))
	for _, f := range files {
		name := strings.ToLower(f.Path)
		if strings.HasPrefix(name, "addons/") || strings.HasPrefix(name, "multiserver/content-staging/") {
			continue
		}
		result = append(result, f)
	}
	return result
}
func BuildInventoryWithOptions(root, output string, options SnapshotOptions) ([]FileEntry, error) {
	files, err := BuildInventory(root, output)
	return filterAddonLibrary(files, options.IncludeAddons), err
}
func inventoryForSnapshotSizing(root, output string, include bool) ([]FileEntry, error) {
	files, err := inventoryForSizing(root, output)
	return filterAddonLibrary(files, include), err
}
