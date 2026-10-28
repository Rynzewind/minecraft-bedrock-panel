package addonupdates

import (
	"path/filepath"
	"testing"
	"time"
)

func TestOverviewStatusNeedsVerifiedEvidence(t *testing.T) {
	v := [3]int{2, 0, 0}
	deployed := []VersionState{{Enabled: true, Known: true, Version: v}}
	src := Source{LastChecked: time.Now(), LatestCheckOK: true, LatestFileID: 3, VerifiedFileID: 3, VerifiedVersion: &v}
	if got := OverviewStatus(v, deployed, Source{}); got != "Unknown" {
		t.Fatal(got)
	}
	if got := OverviewStatus(v, deployed, src); got != "Up to date" {
		t.Fatal(got)
	}
	deployed[0].Version = [3]int{1, 0, 0}
	if got := OverviewStatus(v, deployed, src); got != "Outdated" {
		t.Fatal(got)
	}
	src.LatestFileID = 4
	if got := OverviewStatus(v, deployed, src); got != "Unknown" {
		t.Fatal(got)
	}
	src.LatestFileID = 3
	src.LatestCheckOK = false
	if got := OverviewStatus(v, deployed, src); got != "Unknown" {
		t.Fatal(got)
	}
	src.LatestCheckOK = true
	src.LastChecked = time.Now().Add(-25 * time.Hour)
	if got := OverviewStatus(v, deployed, src); got != "Unknown" {
		t.Fatal(got)
	}
}

func TestSourceIdentityChangeClearsOverviewEvidence(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "sources.json"))
	src := Source{URL: "https://www.curseforge.com/minecraft-bedrock/addons/first", Provider: "curseforge", Channel: "stable"}
	if e := s.SaveRecord("pack", src); e != nil {
		t.Fatal(e)
	}
	if e := s.MarkResult("pack", src, "fetched", 123); e != nil {
		t.Fatal(e)
	}
	if e := s.MarkVerified("pack", src, 123, [3]int{2, 0, 0}); e != nil {
		t.Fatal(e)
	}
	rows, _ := s.ListRecords()
	r := rows["pack"]
	if !r.LatestCheckOK || r.VerifiedVersion == nil {
		t.Fatal("evidence not stored")
	}
	r.Channel = "beta"
	if e := s.SaveRecord("pack", r); e != nil {
		t.Fatal(e)
	}
	rows, _ = s.ListRecords()
	if rows["pack"].LatestCheckOK || rows["pack"].VerifiedVersion != nil {
		t.Fatal("old channel evidence retained")
	}
	if e := s.MarkVerified("pack", src, 123, [3]int{2, 0, 0}); e == nil {
		t.Fatal("stale verification accepted")
	}
}
