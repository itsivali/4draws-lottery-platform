package attest_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"

	"github.com/itsivali/lottery-platform/internal/identity/attest"
)

var now = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

const appID = "ABCDE12345.ke.tajirika.lottery_mobile"

type chain struct {
	root     *x509.Certificate
	rootKey  *ecdsa.PrivateKey
	inter    *x509.Certificate
	interKey *ecdsa.PrivateKey
}

func newChain(t *testing.T) chain {
	t.Helper()
	rk, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	rt := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test App Attestation Root CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	rd, _ := x509.CreateCertificate(rand.Reader, rt, rt, &rk.PublicKey, rk)
	root, _ := x509.ParseCertificate(rd)
	ik, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	it := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "Test App Attestation CA 1"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	id, _ := x509.CreateCertificate(rand.Reader, it, root, &ik.PublicKey, rk)
	inter, _ := x509.ParseCertificate(id)
	return chain{root: root, rootKey: rk, inter: inter, interKey: ik}
}

// attestation builds an attestation object the way an iPhone does: a key pair, a leaf certificate carrying the nonce
// SHA-256(authData ‖ SHA-256(challenge)) in Apple's extension, and authData naming the app and the key.
func (c chain) attestation(t *testing.T, challenge string, mutate func(*fields)) string {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pub, err := key.PublicKey.Bytes() // the uncompressed point Apple hashes
	if err != nil {
		t.Fatal(err)
	}
	keyID := sha256.Sum256(pub)
	f := fields{appID: appID, aaguid: "appattest\x00\x00\x00\x00\x00\x00\x00", counter: 0, credID: keyID[:], challenge: challenge, leafChain: true}
	if mutate != nil {
		mutate(&f)
	}
	rp := sha256.Sum256([]byte(f.appID))
	auth := append([]byte{}, rp[:]...)
	auth = append(auth, 0x41)
	auth = binary.BigEndian.AppendUint32(auth, f.counter)
	auth = append(auth, []byte(f.aaguid)...)
	auth = binary.BigEndian.AppendUint16(auth, uint16(len(f.credID)))
	auth = append(auth, f.credID...)
	cdh := sha256.Sum256([]byte(f.challenge))
	nonce := sha256.Sum256(append(append([]byte{}, auth...), cdh[:]...))
	if f.badNonce {
		nonce[0] ^= 0xff
	}
	ext, _ := asn1.Marshal(struct {
		Nonce []byte `asn1:"tag:1,explicit"`
	}{nonce[:]})
	lt := &x509.Certificate{SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: hex.EncodeToString(keyID[:])},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		ExtraExtensions: []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 8, 2}, Value: ext}}}
	signer, parent := c.interKey, c.inter
	if f.selfSigned {
		other, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		signer, parent = other, &x509.Certificate{SerialNumber: big.NewInt(9), Subject: pkix.Name{CommonName: "Forger"}}
	}
	leaf, _ := x509.CreateCertificate(rand.Reader, lt, parent, &key.PublicKey, signer)
	x5c := [][]byte{leaf, c.inter.Raw}
	obj := map[string]any{"fmt": f.format(), "attStmt": map[string]any{"x5c": x5c, "receipt": []byte("receipt")}, "authData": auth}
	raw, err := cbor.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

type fields struct {
	appID, aaguid, challenge string
	counter                  uint32
	credID                   []byte
	badNonce, selfSigned     bool
	leafChain                bool
	fmt                      string
}

func (f fields) format() string {
	if f.fmt != "" {
		return f.fmt
	}
	return "apple-appattest"
}

func TestAppAttest_AcceptsAGenuineAttestationOfThisAppForThisChallenge(t *testing.T) {
	c := newChain(t)
	v := attest.NewAppAttest(appID, c.root, false, func() time.Time { return now })
	ok, err := v.Verify(c.attestation(t, "challenge-1", nil), "challenge-1")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestAppAttest_RefusesEveryForgeryOrMismatch(t *testing.T) {
	c := newChain(t)
	v := attest.NewAppAttest(appID, c.root, false, func() time.Time { return now })
	for name, tc := range map[string]struct {
		mutate    func(*fields)
		challenge string
		reason    string
	}{
		"another challenge (replay)":    {nil, "challenge-2", "nonce does not match"},
		"another app":                   {func(f *fields) { f.appID = "OTHERTEAM.com.example.app" }, "challenge-1", "another app"},
		"used key (counter not 0)":      {func(f *fields) { f.counter = 1 }, "challenge-1", "already used"},
		"development build":             {func(f *fields) { f.aaguid = "appattestdevelop" }, "challenge-1", "development build"},
		"key id not the certified key":  {func(f *fields) { f.credID = make([]byte, 32) }, "challenge-1", "not the certified key"},
		"nonce does not match":          {func(f *fields) { f.badNonce = true }, "challenge-1", "nonce does not match"},
		"not chained to the Apple root": {func(f *fields) { f.selfSigned = true }, "challenge-1", "chain"},
		"another attestation format":    {func(f *fields) { f.fmt = "packed" }, "challenge-1", "unexpected format"},
	} {
		ok, err := v.Verify(c.attestation(t, "challenge-1", tc.mutate), tc.challenge)
		if ok || err == nil || !strings.Contains(err.Error(), tc.reason) {
			t.Errorf("%s: ok=%v err=%v, want a refusal for %q", name, ok, err, tc.reason)
		}
	}
	for name, tok := range map[string]string{"not base64": "%%%", "not cbor": base64.StdEncoding.EncodeToString([]byte("nope")), "empty": ""} {
		if ok, _ := v.Verify(tok, "challenge-1"); ok {
			t.Errorf("%s: passed", name)
		}
	}
}

func TestAppAttest_DevelopmentBuildsPassOnlyWhenAllowed(t *testing.T) {
	c := newChain(t)
	dev := attest.NewAppAttest(appID, c.root, true, func() time.Time { return now })
	if ok, err := dev.Verify(c.attestation(t, "c", func(f *fields) { f.aaguid = "appattestdevelop" }), "c"); !ok {
		t.Fatalf("development build refused when allowed: %v", err)
	}
}

func TestAppAttest_ExpiredCertificatesAreRefused(t *testing.T) {
	c := newChain(t)
	later := attest.NewAppAttest(appID, c.root, false, func() time.Time { return now.Add(48 * time.Hour) })
	if ok, _ := later.Verify(c.attestation(t, "c", nil), "c"); ok {
		t.Fatal("expired chain accepted")
	}
}

func TestAppleRoot_IsTheOnePublishedByApple(t *testing.T) {
	root := attest.AppleAppAttestationRoot()
	sum := sha256.Sum256(root.Raw)
	if got := hex.EncodeToString(sum[:]); got != "1cb9823ba28ba6ad2d33a006941de2ae4f513ef1d4e831b9f7e0fa7b6242c932" {
		t.Fatalf("root fingerprint %s", got)
	}
	if root.Subject.CommonName != "Apple App Attestation Root CA" {
		t.Fatalf("subject %v", root.Subject)
	}
}

func TestRouter_SendsEachPlatformToItsVerifierAndNeverPassesTheRest(t *testing.T) {
	c := newChain(t)
	r := attest.Router{IOS: attest.NewAppAttest(appID, c.root, false, func() time.Time { return now })}
	if ok, err := r.Verify(t.Context(), "ios", c.attestation(t, "x", nil), "x"); !ok || err != nil {
		t.Fatalf("ios: %v %v", ok, err)
	}
	for _, p := range []string{"android", "web", ""} {
		if ok, err := r.Verify(t.Context(), p, "anything", "x"); ok || err == nil {
			t.Errorf("%q: ok=%v err=%v", p, ok, err)
		}
	}
}
