package opsremote

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func newTestHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()

	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate a host key: %v", err)
	}

	public, err := ssh.NewPublicKey(private.Public())
	if err != nil {
		t.Fatalf("wrap the host key: %v", err)
	}

	return public
}

var testAddr = &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}

func TestOpsRemote_HostKey_AcceptsTheKeyThePinNames(t *testing.T) {
	key := newTestHostKey(t)

	target := Target{Host: "lab", HostKeyFingerprint: ssh.FingerprintSHA256(key)}
	if err := hostKeyCallback(target)("lab:22", testAddr, key); err != nil {
		t.Fatalf("accept the pinned key: %v", err)
	}
}

func TestOpsRemote_HostKey_RefusesAnotherHostKey(t *testing.T) {
	pinned := newTestHostKey(t)
	presented := newTestHostKey(t)

	target := Target{Host: "lab", HostKeyFingerprint: ssh.FingerprintSHA256(pinned)}

	err := hostKeyCallback(target)("lab:22", testAddr, presented)
	if err == nil {
		t.Fatal("a host that presents a different key was accepted")
	}
	if !strings.Contains(err.Error(), ssh.FingerprintSHA256(presented)) {
		t.Fatalf("the refusal does not say what was presented: %v", err)
	}
	if !strings.Contains(err.Error(), ssh.FingerprintSHA256(pinned)) {
		t.Fatalf("the refusal does not say what was pinned: %v", err)
	}
}

func TestOpsRemote_HostKey_AcceptsEveryFormSSHKeygenPrints(t *testing.T) {
	key := newTestHostKey(t)

	sha256 := ssh.FingerprintSHA256(key)
	md5 := ssh.FingerprintLegacyMD5(key)

	tests := []struct {
		name string
		pin  string
	}{
		{name: "sha256 as printed", pin: sha256},
		{name: "sha256 without the prefix", pin: strings.TrimPrefix(sha256, "SHA256:")},
		{name: "sha256 with base64 padding", pin: sha256 + "="},
		{name: "sha256 with surrounding space", pin: " " + sha256 + "\n"},
		{name: "md5 as printed", pin: md5},
		{name: "md5 in lower case", pin: strings.ToLower(md5)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !fingerprintMatches(tc.pin, key) {
				t.Fatalf("pin %q did not match the host key %s", tc.pin, sha256)
			}
		})
	}
}

func TestOpsRemote_HostKey_WithoutAPinAcceptsWhatTheHostPresents(t *testing.T) {
	target := Target{Host: "lab"}

	if err := hostKeyCallback(target)("lab:22", testAddr, newTestHostKey(t)); err != nil {
		t.Fatalf("a target without a pin refused the host key: %v", err)
	}
}
