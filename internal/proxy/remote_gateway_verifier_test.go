package proxy

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func TestGatewaySDPIdentityRequiresIssuedKeyBoundAuthorizedToken(t *testing.T) {
	issuer, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	client, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&client.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	verifier := oidc.NewVerifier("https://issuer.test/", &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&issuer.PublicKey}}, &oidc.Config{ClientID: "minecraft-test", SupportedSigningAlgs: []string{"RS256"}})
	sign := func(key *rsa.PrivateKey, iss, aud, xuid, cpk string, expiry time.Time) string {
		t.Helper()
		signer, e := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
		if e != nil {
			t.Fatal(e)
		}
		token, e := jwt.Signed(signer).Claims(jwt.Claims{Issuer: iss, Audience: jwt.Audience{aud}, Expiry: jwt.NewNumericDate(expiry)}).Claims(map[string]any{"xid": xuid, "cpk": cpk}).Serialize()
		if e != nil {
			t.Fatal(e)
		}
		return token
	}
	allowed := true
	calls := 0
	check := gatewayClientVerifier(verifier, func(x string) bool { calls++; return allowed && x == "123" })
	cpk := base64.StdEncoding.EncodeToString(der)
	good := sign(issuer, "https://issuer.test/", "minecraft-test", "123", cpk, time.Now().Add(time.Hour))
	key, e := check(context.Background(), good)
	if e != nil || !key.Equal(&client.PublicKey) {
		t.Fatal("valid authorized token rejected", e)
	}
	allowed = false
	if _, e = check(context.Background(), good); e == nil {
		t.Fatal("revocation ignored")
	}
	allowed = true
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"unsigned garbage":      "not-a-token",
		"wrong signer":          sign(other, "https://issuer.test/", "minecraft-test", "123", cpk, time.Now().Add(time.Hour)),
		"wrong issuer":          sign(issuer, "https://attacker.test/", "minecraft-test", "123", cpk, time.Now().Add(time.Hour)),
		"wrong audience":        sign(issuer, "https://issuer.test/", "other", "123", cpk, time.Now().Add(time.Hour)),
		"expired":               sign(issuer, "https://issuer.test/", "minecraft-test", "123", cpk, time.Now().Add(-time.Hour)),
		"missing identity":      sign(issuer, "https://issuer.test/", "minecraft-test", "", cpk, time.Now().Add(time.Hour)),
		"unauthorized identity": sign(issuer, "https://issuer.test/", "minecraft-test", "456", cpk, time.Now().Add(time.Hour)),
		"invalid key":           sign(issuer, "https://issuer.test/", "minecraft-test", "123", "bad", time.Now().Add(time.Hour)),
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			before := calls
			if _, e := check(context.Background(), token); e == nil {
				t.Fatal("invalid identity admitted")
			}
			if (name == "wrong signer" || name == "wrong issuer" || name == "wrong audience" || name == "expired") && calls != before {
				t.Fatal("unverified identity reached policy lookup")
			}
		})
	}
}
