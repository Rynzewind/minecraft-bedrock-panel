package authstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sandertv/gophertunnel/minecraft/auth"
	"golang.org/x/oauth2"
)

func TokenSource(ctx context.Context, path string, output io.Writer) (oauth2.TokenSource, error) {
	token, loadErr := Load(path)
	if loadErr == nil {
		source := auth.RefreshTokenSourceWriter(token, output)
		fresh, err := source.Token()
		if err == nil {
			if err := Save(path, fresh); err != nil {
				return nil, err
			}
			return auth.RefreshTokenSourceWriter(fresh, output), nil
		}
		fmt.Fprintln(output, "Cached Microsoft sign-in expired; sign in again.")
	} else if !os.IsNotExist(loadErr) {
		return nil, loadErr
	}

	fmt.Fprintln(output, "Microsoft sign-in is required for the helper's secure connection to BDS.")
	token, err := auth.RequestLiveTokenContext(ctx, output)
	if err != nil {
		return nil, fmt.Errorf("Microsoft device sign-in: %w", err)
	}
	if err := Save(path, token); err != nil {
		return nil, err
	}
	return auth.RefreshTokenSourceWriter(token, output), nil
}

func Load(path string) (*oauth2.Token, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var token oauth2.Token
	if err := json.Unmarshal(b, &token); err != nil {
		return nil, fmt.Errorf("decode cached Microsoft token: %w", err)
	}
	if token.RefreshToken == "" {
		return nil, fmt.Errorf("cached Microsoft token has no refresh token")
	}
	return &token, nil
}

func Save(path string, token *oauth2.Token) error {
	if token == nil || token.RefreshToken == "" {
		return fmt.Errorf("cannot cache Microsoft token without a refresh token")
	}
	b, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("encode Microsoft token: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, b, 0600); err != nil {
		return fmt.Errorf("write temporary token: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("install token: %w", err)
	}
	return nil
}
