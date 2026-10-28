package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/crypto/ssh"
)

type Store struct {
	Directory string
}

func Path(directory string, networkID uint64) string {
	return filepath.Join(directory, strconv.FormatUint(networkID, 10)+".pem")
}

func (s Store) LoadOrCreate(networkID uint64) (*ecdsa.PrivateKey, string, error) {
	if networkID == 0 {
		return nil, "", fmt.Errorf("network ID must be non-zero")
	}
	if err := os.MkdirAll(s.Directory, 0o700); err != nil {
		return nil, "", fmt.Errorf("create identity directory: %w", err)
	}
	path := Path(s.Directory, networkID)
	key, err := load(path)
	if os.IsNotExist(err) {
		key, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		if err == nil {
			err = saveAtomic(path, key)
		}
	}
	if err != nil {
		return nil, "", fmt.Errorf("identity %d: %w", networkID, err)
	}
	if key.Curve != elliptic.P384() {
		return nil, "", fmt.Errorf("identity %d: key is not P-384", networkID)
	}
	publicKey, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		return nil, "", fmt.Errorf("identity %d fingerprint: %w", networkID, err)
	}
	return key, ssh.FingerprintSHA256(publicKey), nil
}

func load(path string) (*ecdsa.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(b)
	if block == nil || block.Type != "EC PRIVATE KEY" || len(rest) != 0 {
		return nil, fmt.Errorf("invalid EC private key PEM")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

func saveAtomic(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".identity-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
