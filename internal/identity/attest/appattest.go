// Package attest verifies device attestation server-side: Apple App Attest on iOS, Google Play Integrity on Android.
// A verdict only ever comes from here; the client sends opaque evidence and a server-issued challenge.
package attest

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/fxamacker/cbor/v2"
)

//go:embed apple_app_attestation_root_ca.pem
var appleRootPEM []byte

// AppleAppAttestationRoot is Apple's App Attestation Root CA
// (https://www.apple.com/certificateauthority/Apple_App_Attestation_Root_CA.pem, SHA-256
// 1cb9823b…6242c932, valid to 2045).
func AppleAppAttestationRoot() *x509.Certificate {
	block, _ := pem.Decode(appleRootPEM)
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		panic("attest: embedded Apple root does not parse: " + err.Error())
	}
	return c
}

var nonceOID = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 8, 2}

const (
	aaguidProduction  = "appattest\x00\x00\x00\x00\x00\x00\x00"
	aaguidDevelopment = "appattestdevelop"
)

// AppAttest verifies App Attest key attestations
// (https://developer.apple.com/documentation/devicecheck/validating-apps-that-connect-to-your-server).
type AppAttest struct {
	appID    string // "<team id>.<bundle id>"
	roots    *x509.CertPool
	allowDev bool
	now      func() time.Time
}

// NewAppAttest returns a verifier for appID that trusts root (AppleAppAttestationRoot in production). allowDev also
// accepts attestations from development builds (never in production).
func NewAppAttest(appID string, root *x509.Certificate, allowDev bool, now func() time.Time) *AppAttest {
	pool := x509.NewCertPool()
	pool.AddCert(root)
	return &AppAttest{appID: appID, roots: pool, allowDev: allowDev, now: now}
}

type attestationObject struct {
	Fmt     string `cbor:"fmt"`
	AttStmt struct {
		X5c [][]byte `cbor:"x5c"`
	} `cbor:"attStmt"`
	AuthData []byte `cbor:"authData"`
}

// Verify checks a base64 attestation object against challenge (the server-issued challenge the app hashed as client
// data). It reports false with an error explaining the first check that failed.
func (v *AppAttest) Verify(token, challenge string) (bool, error) {
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil || len(raw) == 0 {
		return false, errors.New("appattest: not a base64 attestation object")
	}
	var obj attestationObject
	if err := cbor.Unmarshal(raw, &obj); err != nil {
		return false, fmt.Errorf("appattest: decode: %w", err)
	}
	if obj.Fmt != "apple-appattest" {
		return false, errors.New("appattest: unexpected format")
	}
	if len(obj.AttStmt.X5c) < 2 {
		return false, errors.New("appattest: certificate chain missing")
	}
	leaf, err := x509.ParseCertificate(obj.AttStmt.X5c[0])
	if err != nil {
		return false, fmt.Errorf("appattest: leaf: %w", err)
	}
	inter := x509.NewCertPool()
	for _, der := range obj.AttStmt.X5c[1:] {
		c, err := x509.ParseCertificate(der)
		if err != nil {
			return false, fmt.Errorf("appattest: intermediate: %w", err)
		}
		inter.AddCert(c)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: v.roots, Intermediates: inter, CurrentTime: v.now(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); err != nil {
		return false, fmt.Errorf("appattest: chain: %w", err)
	}

	// the nonce Apple certified must be SHA-256(authData ‖ SHA-256(challenge))
	cdh := sha256.Sum256([]byte(challenge))
	want := sha256.Sum256(append(append([]byte{}, obj.AuthData...), cdh[:]...))
	got, err := certifiedNonce(leaf)
	if err != nil {
		return false, err
	}
	if !bytes.Equal(got, want[:]) {
		return false, errors.New("appattest: nonce does not match the challenge")
	}

	pub, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return false, errors.New("appattest: leaf key is not P-256")
	}
	point, err := pub.Bytes() // the uncompressed point, which Apple hashes into the key id
	if err != nil {
		return false, fmt.Errorf("appattest: leaf key: %w", err)
	}
	keyID := sha256.Sum256(point)

	a, err := parseAuthData(obj.AuthData)
	if err != nil {
		return false, err
	}
	rp := sha256.Sum256([]byte(v.appID))
	switch {
	case !bytes.Equal(a.rpIDHash, rp[:]):
		return false, errors.New("appattest: attested for another app")
	case a.counter != 0:
		return false, errors.New("appattest: key already used")
	case a.aaguid == aaguidDevelopment && !v.allowDev:
		return false, errors.New("appattest: development build")
	case a.aaguid != aaguidProduction && a.aaguid != aaguidDevelopment:
		return false, errors.New("appattest: unexpected aaguid")
	case !bytes.Equal(a.credentialID, keyID[:]):
		return false, errors.New("appattest: key id is not the certified key")
	}
	return true, nil
}

func certifiedNonce(leaf *x509.Certificate) ([]byte, error) {
	for _, e := range leaf.Extensions {
		if !e.Id.Equal(nonceOID) {
			continue
		}
		var seq struct {
			Nonce []byte `asn1:"tag:1,explicit"`
		}
		if _, err := asn1.Unmarshal(e.Value, &seq); err != nil {
			return nil, fmt.Errorf("appattest: nonce extension: %w", err)
		}
		return seq.Nonce, nil
	}
	return nil, errors.New("appattest: nonce extension missing")
}

type authData struct {
	rpIDHash     []byte
	counter      uint32
	aaguid       string
	credentialID []byte
}

// parseAuthData reads the WebAuthn-style authenticator data: rpIdHash(32) flags(1) counter(4) aaguid(16)
// credentialIdLength(2) credentialId.
func parseAuthData(b []byte) (authData, error) {
	if len(b) < 55 {
		return authData{}, errors.New("appattest: authenticator data too short")
	}
	n := int(binary.BigEndian.Uint16(b[53:55]))
	if len(b) < 55+n {
		return authData{}, errors.New("appattest: credential id truncated")
	}
	return authData{rpIDHash: b[:32], counter: binary.BigEndian.Uint32(b[33:37]), aaguid: string(b[37:53]), credentialID: b[55 : 55+n]}, nil
}
