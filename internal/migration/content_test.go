package migration

import "testing"

func TestContentPackAssetFoldersAreNotTreatedAsDiagnostics(t *testing.T) {
	root := fixture(t)
	paths := []string{"Creative/resource_packs/p/textures/logs/oak.png", "Creative/behavior_packs/p/data/backups/asset.json", "MultiServer/content-staging/packs/p/logs/asset.json", "Addons/assets/logs/asset.json"}
	for _, p := range paths {
		put(t, root, p, "required-asset")
	}
	files, e := BuildInventory(root, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	seen := map[string]bool{}
	for _, f := range files {
		seen[f.Path] = true
	}
	for _, p := range paths {
		if !seen[p] {
			t.Fatalf("excluded content asset %s", p)
		}
	}
}
