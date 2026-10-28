package addonupdates

import (
	"encoding/json"
	"errors"
	"fmt"
	"nethernet-multi-server/internal/accountstore"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type KeyStatus struct {
	Configured bool      `json:"configured"`
	Valid      bool      `json:"valid"`
	LastTested time.Time `json:"last_tested,omitempty"`
}
type credentialDisk struct {
	Version    int       `json:"version"`
	Cipher     []byte    `json:"encrypted_key"`
	Valid      bool      `json:"valid"`
	LastTested time.Time `json:"last_tested,omitempty"`
}
type Credentials struct {
	mu   sync.Mutex
	path string
	p    accountstore.Protector
}

func NewCredentials(path string, p accountstore.Protector) *Credentials {
	return &Credentials{path: path, p: p}
}
func (c *Credentials) read() (credentialDisk, error) {
	b, e := os.ReadFile(c.path)
	if e != nil {
		return credentialDisk{}, e
	}
	if len(b) > 65536 {
		return credentialDisk{}, fmt.Errorf("invalid credential settings")
	}
	var disk credentialDisk
	if json.Unmarshal(b, &disk) != nil || disk.Version != 1 || len(disk.Cipher) == 0 {
		return disk, fmt.Errorf("saved API key is unreadable; replace it")
	}
	return disk, nil
}
func (c *Credentials) Status() (KeyStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, e := c.read()
	if errors.Is(e, os.ErrNotExist) {
		return KeyStatus{}, nil
	}
	if e != nil {
		return KeyStatus{Configured: true}, fmt.Errorf("saved API key settings cannot be read; replace or remove the key")
	}
	return KeyStatus{Configured: true, Valid: v.Valid, LastTested: v.LastTested}, nil
}
func (c *Credentials) Set(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key = strings.TrimSpace(key)
	if len(key) < 8 || len(key) > 2048 {
		return fmt.Errorf("enter a complete API key")
	}
	for _, r := range key {
		if r < 33 || r > 126 {
			return fmt.Errorf("API key contains unsupported whitespace or characters")
		}
	}
	cipher, e := c.p.Protect([]byte(key))
	if e != nil {
		return fmt.Errorf("Windows could not protect the API key")
	}
	if e = privateDirectory(filepath.Dir(c.path)); e != nil {
		return fmt.Errorf("cannot protect the local API-key directory")
	}
	b, e := json.Marshal(credentialDisk{Version: 1, Cipher: cipher})
	if e != nil {
		return fmt.Errorf("cannot encode protected key settings")
	}
	return atomicWrite(c.path, b)
}
func (c *Credentials) Load() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, e := c.read()
	if errors.Is(e, os.ErrNotExist) {
		return "", fmt.Errorf("add a CurseForge API key in Add-ons settings first")
	}
	if e != nil {
		return "", fmt.Errorf("saved API key cannot be read; replace it")
	}
	plain, e := c.p.Unprotect(v.Cipher)
	if e != nil {
		return "", fmt.Errorf("saved API key belongs to another Windows profile or is damaged; enter it again")
	}
	return string(plain), nil
}
func (c *Credentials) Remove() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := os.Remove(c.path)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return fmt.Errorf("cannot remove saved API key")
	}
	return nil
}
func (c *Credentials) MarkTest(valid bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, e := c.read()
	if e != nil {
		return fmt.Errorf("saved API key is unavailable")
	}
	v.Valid = valid
	v.LastTested = time.Now().UTC()
	b, _ := json.Marshal(v)
	return atomicWrite(c.path, b)
}
