package addonupdates

import "time"

type VersionState struct {
	Enabled, Known bool
	Version        [3]int
}

func OverviewStatus(library [3]int, deployed []VersionState, s Source) string {
	if s.LocallyModified {
		return "Custom / protected"
	}
	if !s.LatestCheckOK || s.LatestFileID <= 0 || s.VerifiedFileID != s.LatestFileID || s.VerifiedVersion == nil || s.LastChecked.IsZero() || time.Since(s.LastChecked) > 24*time.Hour {
		return "Unknown"
	}
	newer := func(a, b [3]int) bool {
		for i := 0; i < 3; i++ {
			if a[i] != b[i] {
				return a[i] > b[i]
			}
		}
		return false
	}
	v := *s.VerifiedVersion
	if newer(v, library) {
		return "Outdated"
	}
	unknown := false
	for _, state := range deployed {
		if !state.Enabled {
			continue
		}
		if !state.Known {
			unknown = true
			continue
		}
		if newer(v, state.Version) {
			return "Outdated"
		}
	}
	if unknown {
		return "Unknown"
	}
	return "Up to date"
}
