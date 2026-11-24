package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"opsagi/pkg/credentials"
	"opsagi/pkg/opsremote"
)

type fakeCredentialSource struct {
	available bool
	records   map[int64]credentials.Credential
	secrets   map[int64][]byte
	used      []int64
	getErr    error
	secretErr error
}

func (f *fakeCredentialSource) Available() bool { return f != nil && f.available }

func (f *fakeCredentialSource) Get(_ context.Context, id int64) (credentials.Credential, error) {
	if f.getErr != nil {
		return credentials.Credential{}, f.getErr
	}

	record, ok := f.records[id]
	if !ok {
		return credentials.Credential{}, credentials.ErrNotFound
	}

	return record, nil
}

func (f *fakeCredentialSource) Secret(_ context.Context, id int64) ([]byte, error) {
	if f.secretErr != nil {
		return nil, f.secretErr
	}

	secret, ok := f.secrets[id]
	if !ok {
		return nil, credentials.ErrNotFound
	}

	return secret, nil
}

func (f *fakeCredentialSource) MarkUsed(_ context.Context, id int64) error {
	f.used = append(f.used, id)

	return nil
}

func newCredentialSource(record credentials.Credential, secret []byte) *fakeCredentialSource {
	return &fakeCredentialSource{
		available: true,
		records:   map[int64]credentials.Credential{record.ID: record},
		secrets:   map[int64][]byte{record.ID: secret},
	}
}

const testPrivateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nstored\n"

func TestOpsTools_Materialize_AKeyCredentialBecomesTheTargetsIdentity(t *testing.T) {
	source := newCredentialSource(credentials.Credential{
		ID: 3, Name: "lab-root", Kind: credentials.KindSSHKey,
		Host: "10.0.0.9", Port: 2222, Username: "ops", HostKeyFingerprint: "SHA256:pin",
	}, []byte(testPrivateKey))

	tools := (&opsTools{}).WithCredentialStore(source)

	target, err := tools.materialize(context.Background(), opsremote.Target{ID: "lab", CredentialID: 3})
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	if string(target.Secret) != testPrivateKey {
		t.Fatalf("secret = %q, want the stored key", target.Secret)
	}
	if target.Password != "" {
		t.Fatal("a key credential also produced a password")
	}
	if target.Host != "10.0.0.9" || target.Port != 2222 || target.User != "ops" {
		t.Fatalf("target = %+v, want the record's host, port and user", target)
	}
	if target.HostKeyFingerprint != "SHA256:pin" {
		t.Fatalf("pin = %q, want the record's pin", target.HostKeyFingerprint)
	}
}

func TestOpsTools_Materialize_WhatTheTargetStatesWins(t *testing.T) {
	source := newCredentialSource(credentials.Credential{
		ID: 4, Name: "lab-root", Kind: credentials.KindSSHKey,
		Host: "10.0.0.9", Port: 22, Username: "root", HostKeyFingerprint: "SHA256:record",
	}, []byte(testPrivateKey))

	tools := (&opsTools{}).WithCredentialStore(source)

	target, err := tools.materialize(context.Background(), opsremote.Target{
		ID: "lab", CredentialID: 4, Host: "10.0.0.5", Port: 2200, User: "deploy", HostKeyFingerprint: "SHA256:target",
	})
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	if target.Host != "10.0.0.5" || target.Port != 2200 || target.User != "deploy" {
		t.Fatalf("target = %+v, want the values the operator wrote", target)
	}
	if target.HostKeyFingerprint != "SHA256:target" {
		t.Fatalf("pin = %q, want the pin the operator wrote", target.HostKeyFingerprint)
	}
}

func TestOpsTools_Materialize_APasswordCredentialFillsThePassword(t *testing.T) {
	source := newCredentialSource(credentials.Credential{
		ID: 5, Name: "lab-pass", Kind: credentials.KindSSHPassword, Host: "10.0.0.10", Username: "root",
	}, []byte("s3cret"))

	tools := (&opsTools{}).WithCredentialStore(source)

	target, err := tools.materialize(context.Background(), opsremote.Target{ID: "lab", CredentialID: 5})
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	if target.Password != "s3cret" {
		t.Fatalf("password = %q, want the stored one", target.Password)
	}
	if len(target.Secret) != 0 {
		t.Fatal("a password credential also produced key material")
	}
}

func TestOpsTools_Materialize_ATokenCannotOpenAnSSHSession(t *testing.T) {
	source := newCredentialSource(credentials.Credential{
		ID: 6, Name: "kube", Kind: credentials.KindK8sToken,
	}, []byte("token"))

	tools := (&opsTools{}).WithCredentialStore(source)

	_, err := tools.materialize(context.Background(), opsremote.Target{ID: "lab", CredentialID: 6})
	if err == nil {
		t.Fatal("a Kubernetes token was accepted as an SSH identity")
	}
	if !strings.Contains(err.Error(), credentials.KindK8sToken) {
		t.Fatalf("err = %v, want it to name the kind", err)
	}
}

func TestOpsTools_Materialize_WithoutAStoreSaysWhichVariableIsMissing(t *testing.T) {
	tools := &opsTools{}

	_, err := tools.materialize(context.Background(), opsremote.Target{ID: "lab", CredentialID: 7})
	if err == nil {
		t.Fatal("a credential target resolved without a store")
	}
	if !strings.Contains(err.Error(), credentials.VarKey) {
		t.Fatalf("err = %v, want it to name %s", err, credentials.VarKey)
	}
}

func TestOpsTools_Materialize_AMissingRecordNamesTheId(t *testing.T) {
	source := &fakeCredentialSource{available: true, records: map[int64]credentials.Credential{}, secrets: map[int64][]byte{}}
	tools := (&opsTools{}).WithCredentialStore(source)

	_, err := tools.materialize(context.Background(), opsremote.Target{ID: "lab", CredentialID: 99})
	if err == nil {
		t.Fatal("a missing credential resolved")
	}
	if !strings.Contains(err.Error(), "99") || !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("err = %v, want it to name 99 and wrap ErrNotFound", err)
	}
}

func TestOpsTools_Materialize_ATargetWithoutACredentialIsLeftAlone(t *testing.T) {
	tools := &opsTools{}

	target, err := tools.materialize(context.Background(), opsremote.Target{ID: "lab", Host: "10.0.0.9", Password: "pw"})
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if target.Host != "10.0.0.9" || target.Password != "pw" {
		t.Fatalf("target = %+v, want it unchanged", target)
	}
}

func TestOpsTools_Dial_RefusesACredentialTargetBeforeItDials(t *testing.T) {
	tools := &opsTools{
		targets: map[string]opsremote.Target{"lab": {ID: "lab", Host: "10.0.0.9", CredentialID: 9}},
		order:   []string{"lab"},
		timeout: time.Second,
	}

	args, err := json.Marshal(OpsTargetArgs{Target: "lab"})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	_, _, err = tools.dial(context.Background(), args)
	if err == nil {
		t.Fatal("dial proceeded without a credential store")
	}
	if !strings.Contains(err.Error(), credentials.VarKey) {
		t.Fatalf("err = %v, want it to name %s before any connection", err, credentials.VarKey)
	}
	if strings.Contains(err.Error(), "连接目标主机") {
		t.Fatalf("err = %v, want the refusal before the dial", err)
	}
}
