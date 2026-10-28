package serverupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const metadataURL = "https://net-secondary.web.minecraft-services.net/api/v1.0/download/links"

type Release struct {
	Version       string `json:"version"`
	URL           string `json:"url"`
	Compatible    bool   `json:"compatible"`
	HelperVersion string `json:"helper_version"`
	Protocol      int32  `json:"protocol"`
	Reason        string `json:"reason,omitempty"`
}

var releasePath = regexp.MustCompile(`^/bedrockdedicatedserver/bin-win/bedrock-server-(1\.[0-9]+\.[0-9]+\.[0-9]+)\.zip$`)

func parseRelease(raw string) (Release, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host != "www.minecraft.net" || u.RawQuery != "" || u.Fragment != "" {
		return Release{}, fmt.Errorf("untrusted server download URL")
	}
	m := releasePath.FindStringSubmatch(u.Path)
	if m == nil {
		return Release{}, fmt.Errorf("not an official stable Windows server archive")
	}
	r := Release{Version: m[1], URL: raw, HelperVersion: protocol.CurrentVersion, Protocol: protocol.CurrentProtocol}
	r.Compatible = strings.Join(strings.Split(r.Version, ".")[:3], ".") == protocol.CurrentVersion
	if !r.Compatible {
		r.Reason = "Update NetherNet first: this release requires a different Minecraft protocol. No server files will be changed."
	}
	return r, nil
}
func officialClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return fmt.Errorf("too many redirects")
		}
		if req.URL.Scheme != "https" || req.URL.Host != "www.minecraft.net" {
			return fmt.Errorf("untrusted download redirect")
		}
		return nil
	}}
}
func get(ctx context.Context, client *http.Client, address string, limit int64) ([]byte, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", address, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", "NetherNet-ServerUpdater/1")
	resp, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("download exceeds size limit")
	}
	return b, e
}
func latest(ctx context.Context) (Release, error) {
	b, e := get(ctx, officialClient(), metadataURL, 2<<20)
	if e != nil {
		return Release{}, e
	}
	var v struct {
		Result struct {
			Links []struct {
				Type string `json:"downloadType"`
				URL  string `json:"downloadUrl"`
			} `json:"links"`
		} `json:"result"`
	}
	if e = json.Unmarshal(b, &v); e != nil {
		return Release{}, e
	}
	for _, l := range v.Result.Links {
		if l.Type == "serverBedrockWindows" {
			return parseRelease(l.URL)
		}
	}
	return Release{}, fmt.Errorf("official stable Windows download not found")
}
