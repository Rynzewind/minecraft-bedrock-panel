package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/auth"
	"golang.org/x/oauth2"

	"nethernet-multi-server/internal/accountstore"
)

type DeviceCode struct {
	DeviceCode              string    `json:"-"`
	UserCode                string    `json:"user_code"`
	VerificationURI         string    `json:"verification_uri"`
	VerificationURIComplete string    `json:"verification_uri_complete,omitempty"`
	Expiry                  time.Time `json:"expiry"`
	Interval                int64     `json:"interval"`
}
type XboxIdentity struct {
	XUID     string `json:"xuid"`
	Gamertag string `json:"gamertag"`
}
type Labels struct {
	Role   string `json:"role,omitempty"`
	Person string `json:"person"`
	Email  string `json:"email,omitempty"`
}
type Authenticator interface {
	Begin(context.Context) (DeviceCode, error)
	Complete(context.Context, DeviceCode) (*oauth2.Token, XboxIdentity, error)
	Validate(context.Context, *oauth2.Token) (XboxIdentity, error)
}

type LiveAuthenticator struct{}

func (LiveAuthenticator) Begin(ctx context.Context) (DeviceCode, error) {
	d, err := auth.AndroidConfig.DeviceAuth(ctx)
	if err != nil {
		return DeviceCode{}, err
	}
	return DeviceCode{DeviceCode: d.DeviceCode, UserCode: d.UserCode, VerificationURI: d.VerificationURI, VerificationURIComplete: d.VerificationURIComplete, Expiry: d.Expiry, Interval: d.Interval}, nil
}
func (LiveAuthenticator) Complete(ctx context.Context, d DeviceCode) (*oauth2.Token, XboxIdentity, error) {
	od := &oauth2.DeviceAuthResponse{DeviceCode: d.DeviceCode, UserCode: d.UserCode, VerificationURI: d.VerificationURI, VerificationURIComplete: d.VerificationURIComplete, Expiry: d.Expiry, Interval: d.Interval}
	t, err := auth.AndroidConfig.DeviceAccessToken(ctx, od)
	if err != nil {
		return nil, XboxIdentity{}, err
	}
	id, err := (LiveAuthenticator{}).Validate(ctx, t)
	return t, id, err
}
func (LiveAuthenticator) Validate(ctx context.Context, t *oauth2.Token) (XboxIdentity, error) {
	if t == nil || t.RefreshToken == "" {
		return XboxIdentity{}, errors.New("Microsoft token has no refresh token")
	}
	fresh, err := auth.AndroidConfig.TokenSource(ctx, t).Token()
	if err != nil {
		return XboxIdentity{}, fmt.Errorf("refresh Microsoft token: %w", err)
	}
	xbl, err := auth.AndroidConfig.RequestXBLToken(ctx, fresh, "http://xboxlive.com")
	if err != nil {
		return XboxIdentity{}, fmt.Errorf("validate Xbox identity: %w", err)
	}
	if !xbl.Valid() {
		return XboxIdentity{}, errors.New("Xbox identity token is invalid")
	}
	info := xbl.AuthorizationToken.UserInfo()
	if info.XUID == "" || info.GamerTag == "" {
		return XboxIdentity{}, errors.New("Xbox identity is missing XUID or gamertag")
	}
	return XboxIdentity{XUID: info.XUID, Gamertag: info.GamerTag}, nil
}

type Enrollment struct {
	store  *accountstore.Store
	auth   Authenticator
	labels Labels
	code   DeviceCode
}

func BeginEnrollment(ctx context.Context, store *accountstore.Store, a Authenticator, labels Labels) (DeviceCode, *Enrollment, error) {
	if strings.TrimSpace(labels.Person) == "" {
		return DeviceCode{}, nil, errors.New("person label is required")
	}
	if err := accountstore.ValidateRole(labels.Role); err != nil {
		return DeviceCode{}, nil, err
	}
	code, err := a.Begin(ctx)
	if err != nil {
		return DeviceCode{}, nil, err
	}
	return code, &Enrollment{store: store, auth: a, labels: labels, code: code}, nil
}
func (e *Enrollment) Complete(ctx context.Context) (accountstore.Account, error) {
	t, id, err := e.auth.Complete(ctx, e.code)
	if err != nil {
		return accountstore.Account{}, err
	}
	return addValidated(e.store, t, id, e.labels)
}

func (e *Enrollment) CompleteReplacement(ctx context.Context, accountID string) (accountstore.Account, error) {
	t, id, err := e.auth.Complete(ctx, e.code)
	if err != nil {
		return accountstore.Account{}, err
	}
	b, err := json.Marshal(t)
	if err != nil {
		return accountstore.Account{}, err
	}
	snapshot, err := e.store.Load()
	if err != nil {
		return accountstore.Account{}, err
	}
	for i := range snapshot.Accounts {
		if snapshot.Accounts[i].ID != accountID {
			continue
		}
		if snapshot.Accounts[i].XUID != id.XUID {
			return accountstore.Account{}, errors.New("the signed-in Xbox account does not match the selected pool account")
		}
		snapshot.Accounts[i].EncryptedToken = string(b)
		snapshot.Accounts[i].Gamertag = id.Gamertag
		snapshot.Accounts[i].Healthy = true
		if err := e.store.Save(snapshot); err != nil {
			return accountstore.Account{}, err
		}
		result := snapshot.Accounts[i]
		result.EncryptedToken = ""
		return result, nil
	}
	return accountstore.Account{}, fmt.Errorf("account %q not found", accountID)
}
func addValidated(store *accountstore.Store, t *oauth2.Token, id XboxIdentity, labels Labels) (accountstore.Account, error) {
	if err := accountstore.ValidateRole(labels.Role); err != nil {
		return accountstore.Account{}, err
	}
	b, err := json.Marshal(t)
	if err != nil {
		return accountstore.Account{}, err
	}
	snap, err := store.Load()
	if err != nil {
		return accountstore.Account{}, err
	}
	for _, a := range snap.Accounts {
		if a.XUID == id.XUID {
			return accountstore.Account{}, fmt.Errorf("Xbox account %s is already enrolled", id.Gamertag)
		}
	}
	a := accountstore.Account{Role: accountstore.EffectiveRole(labels.Role), ID: uuid.NewString(), PersonLabel: strings.TrimSpace(labels.Person), EmailLabel: strings.TrimSpace(labels.Email), Gamertag: id.Gamertag, XUID: id.XUID, EncryptedToken: string(b), Order: len(snap.Accounts), Healthy: true}
	snap.Accounts = append(snap.Accounts, a)
	if err = store.Save(snap); err != nil {
		return accountstore.Account{}, err
	}
	return a, nil
}
func TokenSource(a accountstore.Account) (oauth2.TokenSource, error) {
	var t oauth2.Token
	if err := json.Unmarshal([]byte(a.EncryptedToken), &t); err != nil {
		return nil, fmt.Errorf("decode account token: %w", err)
	}
	if t.RefreshToken == "" {
		return nil, errors.New("account token has no refresh token")
	}
	return auth.AndroidConfig.RefreshTokenSource(&t), nil
}

func MigrateLegacyToken(ctx context.Context, configDir string, store *accountstore.Store, a Authenticator) error {
	path := filepath.Join(configDir, "auth-token.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var t oauth2.Token
	if err = json.Unmarshal(b, &t); err != nil {
		return fmt.Errorf("decode legacy token: %w", err)
	}
	id, err := a.Validate(ctx, &t)
	if err != nil {
		return err
	}
	snap, err := store.Load()
	if err != nil {
		return err
	}
	for _, existing := range snap.Accounts {
		if existing.XUID == id.XUID {
			return renameLegacyBackup(path, configDir)
		}
	}
	if _, err = addValidated(store, &t, id, Labels{Person: "Existing helper account"}); err != nil {
		return err
	}
	if _, err = store.Load(); err != nil {
		return fmt.Errorf("verify migrated account store: %w", err)
	}
	if err = renameLegacyBackup(path, configDir); err != nil {
		return fmt.Errorf("rename migrated token backup: %w", err)
	}
	return nil
}

func renameLegacyBackup(path, configDir string) error {
	backup := filepath.Join(configDir, "auth-token.migrated-backup.json")
	if _, err := os.Stat(backup); err == nil {
		backup = filepath.Join(configDir, "auth-token.migrated-backup-"+time.Now().UTC().Format("20060102-150405")+".json")
	}
	return os.Rename(path, backup)
}
