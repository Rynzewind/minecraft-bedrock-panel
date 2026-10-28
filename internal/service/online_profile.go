package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sandertv/gophertunnel/minecraft/auth"
	"io"
	"net/http"
	"net/url"
	"nethernet-multi-server/internal/online"
	"strings"
	"time"
)

func (c *Controller) resolveOnlineGamertag(ctx context.Context, accountID, gamertag string) (online.Player, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	source, e := c.manager.BroadcasterTokenSource(ctx, accountID)
	if e != nil {
		return online.Player{}, fmt.Errorf("broadcaster authentication required; use Accounts")
	}
	token, e := source.Token()
	if e != nil {
		return online.Player{}, fmt.Errorf("broadcaster authentication required; use Accounts")
	}
	xbox, e := auth.AndroidConfig.RequestXBLToken(ctx, token, "http://xboxlive.com")
	if e != nil {
		return online.Player{}, fmt.Errorf("Xbox identity lookup authentication failed; reauthenticate in Accounts")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", "https://profile.xboxlive.com/users/gt("+url.PathEscape(gamertag)+")/profile/settings?settings=Gamertag", nil)
	if e != nil {
		return online.Player{}, fmt.Errorf("invalid gamertag")
	}
	xbox.SetAuthHeader(req)
	req.Header.Set("x-xbl-contract-version", "2")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("redirect refused") }}
	resp, e := client.Do(req)
	if e != nil {
		return online.Player{}, fmt.Errorf("Xbox profile lookup failed; check connectivity")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return online.Player{}, fmt.Errorf("Xbox profile lookup unavailable (HTTP %d); no access granted", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if e != nil || len(b) > 65536 {
		return online.Player{}, fmt.Errorf("Xbox profile response unavailable or exceeds limit")
	}
	return parseOnlineProfile(b, gamertag)
}
func parseOnlineProfile(b []byte, expected string) (online.Player, error) {
	var out struct {
		Users []struct {
			ID       string `json:"id"`
			Settings []struct{ ID, Value string }
		} `json:"profileUsers"`
	}
	if json.Unmarshal(b, &out) != nil || len(out.Users) != 1 {
		return online.Player{}, fmt.Errorf("Xbox profile did not resolve uniquely")
	}
	user := out.Users[0]
	name := ""
	for _, s := range user.Settings {
		if s.ID == "Gamertag" {
			name = s.Value
		}
	}
	if user.ID == "" || len(user.ID) > 20 || strings.Trim(user.ID, "0123456789") != "" || user.ID == "0" || name == "" || !strings.EqualFold(name, strings.TrimSpace(expected)) {
		return online.Player{}, fmt.Errorf("Xbox identity did not match the requested gamertag; no access granted")
	}
	return online.Player{XUID: user.ID, Gamertag: name, Permissions: map[string]bool{}}, nil
}
