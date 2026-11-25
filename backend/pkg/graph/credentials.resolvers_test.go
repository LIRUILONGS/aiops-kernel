package graph

import (
	"context"
	"errors"
	"testing"
	"time"

	"opsagi/pkg/credentials"
	"opsagi/pkg/graph/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// graphCredentialStore stands in for the sealed store. It records what the
// resolvers asked it to write so a test can check the values that never come
// back out through the API.
type graphCredentialStore struct {
	available bool

	listed  []credentials.Credential
	getByID map[int64]credentials.Credential

	created []credentials.Input
	rotated map[int64][]byte
	deleted []int64

	listErr   error
	createErr error
	rotateErr error
	deleteErr error

	listCalls int
}

func (s *graphCredentialStore) Available() bool { return s.available }

func (s *graphCredentialStore) List(context.Context) ([]credentials.Credential, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}

	return s.listed, nil
}

func (s *graphCredentialStore) Get(_ context.Context, id int64) (credentials.Credential, error) {
	if row, ok := s.getByID[id]; ok {
		return row, nil
	}

	return credentials.Credential{}, credentials.ErrNotFound
}

func (s *graphCredentialStore) Create(_ context.Context, in credentials.Input) (credentials.Credential, error) {
	if s.createErr != nil {
		return credentials.Credential{}, s.createErr
	}

	s.created = append(s.created, in)

	return credentials.Credential{
		ID:                 7,
		Name:               in.Name,
		Kind:               in.Kind,
		Host:               in.Host,
		Port:               in.Port,
		Username:           in.Username,
		HostKeyFingerprint: in.HostKeyFingerprint,
		RotatedAt:          time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC),
		CreatedAt:          time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC),
	}, nil
}

func (s *graphCredentialStore) Rotate(_ context.Context, id int64, secret []byte) error {
	if s.rotateErr != nil {
		return s.rotateErr
	}

	if s.rotated == nil {
		s.rotated = make(map[int64][]byte)
	}

	s.rotated[id] = secret

	return nil
}

func (s *graphCredentialStore) Delete(_ context.Context, id int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}

	s.deleted = append(s.deleted, id)

	return nil
}

func credentialRow() credentials.Credential {
	used := time.Date(2026, 11, 26, 8, 30, 0, 0, time.UTC)

	return credentials.Credential{
		ID:                 3,
		Name:               "lab-root",
		Kind:               credentials.KindSSHPassword,
		Host:               "10.0.0.5",
		Port:               22,
		Username:           "root",
		HostKeyFingerprint: "SHA256:abc",
		RotatedAt:          time.Date(2026, 11, 20, 7, 0, 0, 0, time.UTC),
		LastUsedAt:         &used,
		CreatedAt:          time.Date(2026, 11, 1, 7, 0, 0, 0, time.UTC),
	}
}

func TestCredentials_Credentials_ReturnsTheMetadataButNoSecret(t *testing.T) {
	store := &graphCredentialStore{available: true, listed: []credentials.Credential{credentialRow()}}
	resolver := graphResolver(&graphDB{})
	resolver.CredentialStore = store

	got, err := resolver.Query().Credentials(graphSessionContext("resources.view"))
	require.NoError(t, err)
	require.Len(t, got, 1)

	want := credentialRow()
	assert.Equal(t, want.ID, got[0].ID)
	assert.Equal(t, want.Name, got[0].Name)
	assert.Equal(t, model.CredentialKind(credentials.KindSSHPassword), got[0].Kind)
	assert.Equal(t, want.Host, got[0].Host)
	assert.Equal(t, want.Port, got[0].Port)
	assert.Equal(t, want.HostKeyFingerprint, got[0].HostKeyFingerprint)
	require.NotNil(t, got[0].LastUsedAt)
	assert.Equal(t, want.LastUsedAt.UTC(), got[0].LastUsedAt.UTC())
}

func TestCredentials_Credentials_RefusesADeploymentWithoutAMasterKey(t *testing.T) {
	t.Run("no store wired", func(t *testing.T) {
		resolver := graphResolver(&graphDB{})

		_, err := resolver.Query().Credentials(graphSessionContext("resources.view"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "master key")
	})

	t.Run("store without a sealer", func(t *testing.T) {
		store := &graphCredentialStore{available: false}
		resolver := graphResolver(&graphDB{})
		resolver.CredentialStore = store

		_, err := resolver.Query().Credentials(graphSessionContext("resources.view"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "master key")
		assert.Equal(t, 0, store.listCalls, "an unavailable store must not be read")
	})
}

func TestCredentials_Credentials_RefusesACallerWithoutTheViewPermission(t *testing.T) {
	store := &graphCredentialStore{available: true}
	resolver := graphResolver(&graphDB{})
	resolver.CredentialStore = store

	_, err := resolver.Query().Credentials(graphSessionContext())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrForbidden))
	assert.Equal(t, 0, store.listCalls)
}

func TestCredentials_CreateCredential_StoresTheSealedInputAndReturnsItsMetadata(t *testing.T) {
	store := &graphCredentialStore{available: true}
	resolver := graphResolver(&graphDB{})
	resolver.CredentialStore = store

	port := 2222
	got, err := resolver.Mutation().CreateCredential(graphSessionContext("resources.admin"), model.CredentialCreateInput{
		Name:               "lab-root",
		Kind:               model.CredentialKindSSHKey,
		Secret:             "-----BEGIN OPENSSH PRIVATE KEY-----",
		Host:               strPtr("10.0.0.5"),
		Port:               &port,
		Username:           strPtr("root"),
		HostKeyFingerprint: strPtr("SHA256:abc"),
	})
	require.NoError(t, err)

	require.Len(t, store.created, 1)
	written := store.created[0]
	assert.Equal(t, "lab-root", written.Name)
	assert.Equal(t, credentials.KindSSHKey, written.Kind)
	assert.Equal(t, "-----BEGIN OPENSSH PRIVATE KEY-----", string(written.Secret), "the plaintext must reach the sealer")
	assert.Equal(t, "10.0.0.5", written.Host)
	assert.Equal(t, 2222, written.Port)
	assert.Equal(t, "root", written.Username)
	assert.Equal(t, "SHA256:abc", written.HostKeyFingerprint)
	assert.Equal(t, int64(1), written.CreatedBy)

	require.NotNil(t, got)
	assert.Equal(t, "lab-root", got.Name)
	assert.Equal(t, model.CredentialKindSSHKey, got.Kind)
}

func TestCredentials_CreateCredential_TreatsAnOmittedHostAsEmpty(t *testing.T) {
	store := &graphCredentialStore{available: true}
	resolver := graphResolver(&graphDB{})
	resolver.CredentialStore = store

	_, err := resolver.Mutation().CreateCredential(graphSessionContext("resources.admin"), model.CredentialCreateInput{
		Name:   "k8s",
		Kind:   model.CredentialKindK8sToken,
		Secret: "token",
	})
	require.NoError(t, err)

	require.Len(t, store.created, 1)
	assert.Equal(t, "", store.created[0].Host)
	assert.Equal(t, 0, store.created[0].Port)
	assert.Equal(t, "", store.created[0].Username)
}

func TestCredentials_CreateCredential_RefusesAPortOutsideTheRange(t *testing.T) {
	store := &graphCredentialStore{available: true}
	resolver := graphResolver(&graphDB{})
	resolver.CredentialStore = store

	port := 70000
	_, err := resolver.Mutation().CreateCredential(graphSessionContext("resources.admin"), model.CredentialCreateInput{
		Name:   "lab-root",
		Kind:   model.CredentialKindSSHPassword,
		Secret: "hunter2",
		Port:   &port,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "port")
	assert.Empty(t, store.created)
}

func TestCredentials_CreateCredential_RefusesACallerWithoutTheAdminPermission(t *testing.T) {
	store := &graphCredentialStore{available: true}
	resolver := graphResolver(&graphDB{})
	resolver.CredentialStore = store

	_, err := resolver.Mutation().CreateCredential(graphSessionContext("resources.view"), model.CredentialCreateInput{
		Name:   "lab-root",
		Kind:   model.CredentialKindSSHPassword,
		Secret: "hunter2",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrForbidden))
	assert.Empty(t, store.created)
}

func TestCredentials_RotateCredential_ReportsTheRowItReadBack(t *testing.T) {
	rotated := credentialRow()
	rotated.RotatedAt = time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)

	store := &graphCredentialStore{
		available: true,
		getByID:   map[int64]credentials.Credential{3: rotated},
	}
	resolver := graphResolver(&graphDB{})
	resolver.CredentialStore = store

	got, err := resolver.Mutation().RotateCredential(
		graphSessionContext("resources.admin"),
		3,
		model.CredentialRotateInput{Secret: "new-secret"},
	)
	require.NoError(t, err)

	assert.Equal(t, []byte("new-secret"), store.rotated[3])
	require.NotNil(t, got)
	assert.Equal(t, int64(3), got.ID)
	assert.Equal(t, rotated.RotatedAt.UTC(), got.RotatedAt.UTC())
}

func TestCredentials_RotateCredential_ReportsAMissingRow(t *testing.T) {
	store := &graphCredentialStore{available: true}
	resolver := graphResolver(&graphDB{})
	resolver.CredentialStore = store

	_, err := resolver.Mutation().RotateCredential(
		graphSessionContext("resources.admin"),
		99,
		model.CredentialRotateInput{Secret: "new-secret"},
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, credentials.ErrNotFound))
}

func TestCredentials_DeleteCredential_RemovesTheRowAndReturnsTrue(t *testing.T) {
	store := &graphCredentialStore{available: true}
	resolver := graphResolver(&graphDB{})
	resolver.CredentialStore = store

	ok, err := resolver.Mutation().DeleteCredential(graphSessionContext("resources.admin"), 5)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, []int64{5}, store.deleted)
}

func TestCredentials_DeleteCredential_RefusesACallerWithoutTheAdminPermission(t *testing.T) {
	store := &graphCredentialStore{available: true}
	resolver := graphResolver(&graphDB{})
	resolver.CredentialStore = store

	ok, err := resolver.Mutation().DeleteCredential(graphSessionContext("resources.view"), 5)
	require.Error(t, err)
	assert.False(t, ok)
	assert.Empty(t, store.deleted)
}
