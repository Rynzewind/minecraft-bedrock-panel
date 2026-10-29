package proxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"errors"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/sandertv/gophertunnel/minecraft/service"
)

func newGatewayClientVerifier(ctx context.Context, authorize func(string) bool) (func(context.Context, string) (*ecdsa.PublicKey, error), error) {
	discovery, err := service.Default(ctx)
	if err != nil {
		return nil, errors.New("gateway Minecraft discovery unavailable")
	}
	env := new(service.AuthorizationEnvironment)
	if discovery.Environment(env) != nil {
		return nil, errors.New("gateway Minecraft authentication environment unavailable")
	}
	verifier, err := env.VerifierContext(ctx)
	if err != nil {
		return nil, errors.New("gateway Minecraft token verifier unavailable")
	}
	return gatewayClientVerifier(verifier, authorize), nil
}

// Verify issuer, audience, expiry and signature before using the XUID or cpk.
// NetherNet then verifies the detached fingerprint assertion with this key.
// The Minecraft Login hook independently authenticates and rechecks the ACL.
func gatewayClientVerifier(verifier *oidc.IDTokenVerifier, authorize func(string) bool) func(context.Context, string) (*ecdsa.PublicKey, error) {
	return func(ctx context.Context, raw string) (*ecdsa.PublicKey, error) {
		denied := errors.New("gateway client authentication or authorization failed")
		if verifier == nil || authorize == nil {
			return nil, denied
		}
		token, err := verifier.Verify(ctx, raw)
		if err != nil {
			return nil, denied
		}
		var claims struct {
			XUID      string `json:"xid"`
			PublicKey string `json:"cpk"`
		}
		if token.Claims(&claims) != nil || claims.XUID == "" || !authorize(claims.XUID) {
			return nil, denied
		}
		der, err := base64.StdEncoding.DecodeString(claims.PublicKey)
		if err != nil {
			return nil, denied
		}
		parsed, err := x509.ParsePKIXPublicKey(der)
		if err != nil {
			return nil, denied
		}
		key, ok := parsed.(*ecdsa.PublicKey)
		if !ok || key.Curve != elliptic.P384() || !key.Curve.IsOnCurve(key.X, key.Y) {
			return nil, denied
		}
		return key, nil
	}
}
