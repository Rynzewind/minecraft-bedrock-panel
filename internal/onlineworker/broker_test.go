package onlineworker

import (
	"context"
	"encoding/json"
	"golang.org/x/oauth2"
	"io"
	"strings"
	"testing"
	"time"
)

func TestBrokerBindsRequestsAndStripsRefreshCredentials(t *testing.T) {
	in, childOut := io.Pipe()
	childIn, out := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	cfg := Bootstrap{Type: "bootstrap", Version: 1, ServerID: "creative", AccountID: "dedicated", GatewayHost: "play.test.net", GatewayPort: 19140, WorldName: "Creative", ExpectedProtocol: 2225, AllowUnknownFriendsRoute: true}
	calls := 0
	go func() {
		done <- Serve(ctx, in, out, cfg, Callbacks{Token: func(context.Context) (*oauth2.Token, error) {
			calls++
			return &oauth2.Token{AccessToken: "access-secret", RefreshToken: "never-export", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}, nil
		}, Authorize: func(x string) bool { return x == "123" }})
	}()
	dec := json.NewDecoder(childIn)
	var boot map[string]any
	if err := dec.Decode(&boot); err != nil {
		t.Fatal(err)
	}
	io.WriteString(childOut, "{\"type\":\"token_request\",\"id\":1,\"server_id\":\"creative\",\"account_id\":\"dedicated\"}\n")
	var response json.RawMessage
	if err := dec.Decode(&response); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(response), "never-export") || strings.Contains(string(response), "refresh_token") || !strings.Contains(string(response), "access-secret") {
		t.Fatalf("credential boundary failed")
	}
	io.WriteString(childOut, "{\"type\":\"token_request\",\"id\":2,\"server_id\":\"survival\",\"account_id\":\"dedicated\"}\n")
	if err := <-done; err == nil {
		t.Fatal("cross-server request accepted")
	}
	if calls != 1 {
		t.Fatal(calls)
	}
	childIn.Close()
	childOut.Close()
}

func TestBrokerRejectsUnknownEventsWithoutEcho(t *testing.T) {
	err := Serve(context.Background(), strings.NewReader("{\"type\":\"event\",\"code\":\"token-secret\"}\n"), io.Discard, Bootstrap{}, Callbacks{})
	if err == nil || strings.Contains(err.Error(), "token-secret") {
		t.Fatal("untrusted output exposed")
	}
}
