package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"opsagi/pkg/credentials"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCredentialStore struct {
	available bool
	records   []credentials.Credential
	created   []credentials.Input
	rotated   map[int64][]byte
	deleted   []int64
	createErr error
	rotateErr error
}

func (f *fakeCredentialStore) Available() bool { return f.available }

func (f *fakeCredentialStore) List(_ context.Context) ([]credentials.Credential, error) {
	return f.records, nil
}

func (f *fakeCredentialStore) Create(_ context.Context, in credentials.Input) (credentials.Credential, error) {
	if f.createErr != nil {
		return credentials.Credential{}, f.createErr
	}
	f.created = append(f.created, in)

	return credentials.Credential{
		ID: 12, Name: in.Name, Kind: in.Kind, Host: in.Host, Port: in.Port, Username: in.Username,
		RotatedAt: time.Date(2026, 11, 24, 6, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2026, 11, 24, 6, 0, 0, 0, time.UTC),
	}, nil
}

func (f *fakeCredentialStore) Rotate(_ context.Context, id int64, secret []byte) error {
	if f.rotateErr != nil {
		return f.rotateErr
	}
	if f.rotated == nil {
		f.rotated = make(map[int64][]byte)
	}
	f.rotated[id] = secret

	return nil
}

func (f *fakeCredentialStore) Delete(_ context.Context, id int64) error {
	f.deleted = append(f.deleted, id)

	return nil
}

func credentialRequest(t *testing.T, store credentialStore, handler func(*CredentialService, *gin.Context),
	method, target, body string, privileges []string,
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	service := NewCredentialService(store)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("uid", uint64(7))
	c.Set("prm", privileges)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if strings.HasSuffix(target, "/secret") || method == http.MethodDelete {
		c.Params = gin.Params{{Key: "id", Value: "12"}}
	}

	handler(service, c)

	return w
}

func credentialErrorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	var resp struct {
		Status string `json:"status"`
		Code   string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	return resp.Code
}

const adminPrivileges = "settings.admin"

func TestCredentials_CreateCredential_AnswersWithTheMetadataAndNeverTheSecret(t *testing.T) {
	store := &fakeCredentialStore{available: true}
	const secret = "-----BEGIN OPENSSH PRIVATE KEY-----never-echo-this"

	w := credentialRequest(t, store, (*CredentialService).CreateCredential, http.MethodPost, "/credentials/",
		`{"name":"lab-root","kind":"ssh_key","secret":"`+secret+`","host":"10.0.0.9","port":2222,"username":"root"}`,
		[]string{adminPrivileges})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "never-echo-this", "the response carried the secret back")

	var resp struct {
		Data struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Kind string `json:"kind"`
			Port int    `json:"port"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, int64(12), resp.Data.ID)
	assert.Equal(t, "lab-root", resp.Data.Name)
	assert.Equal(t, "ssh_key", resp.Data.Kind)
	assert.Equal(t, 2222, resp.Data.Port)
	require.Len(t, store.created, 1)
	assert.Equal(t, secret, string(store.created[0].Secret))
	assert.Equal(t, int64(7), store.created[0].CreatedBy, "the row records who stored it")
}

func TestCredentials_CreateCredential_RefusesAKindNothingCanOpen(t *testing.T) {
	store := &fakeCredentialStore{available: true}

	w := credentialRequest(t, store, (*CredentialService).CreateCredential, http.MethodPost, "/credentials/",
		`{"name":"x","kind":"kerberos","secret":"s"}`, []string{adminPrivileges})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "Credentials.InvalidRequest", credentialErrorCode(t, w))
	assert.Empty(t, store.created, "an unstorable credential reached the store")
}

func TestCredentials_GetCredentials_RefusesAUserWithoutThePrivilege(t *testing.T) {
	store := &fakeCredentialStore{available: true}

	w := credentialRequest(t, store, (*CredentialService).GetCredentials, http.MethodGet, "/credentials/",
		"", []string{"flows.view"})

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "NotPermitted", credentialErrorCode(t, w))
}

func TestCredentials_GetCredentials_Answers503WithoutAMasterKey(t *testing.T) {
	store := &fakeCredentialStore{available: false}

	w := credentialRequest(t, store, (*CredentialService).GetCredentials, http.MethodGet, "/credentials/",
		"", []string{adminPrivileges})

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "Credentials.Unavailable", credentialErrorCode(t, w))
}

func TestCredentials_GetCredentials_ListsWhatTheStoreHolds(t *testing.T) {
	store := &fakeCredentialStore{available: true, records: []credentials.Credential{
		{ID: 12, Name: "lab-root", Kind: credentials.KindSSHKey, Host: "10.0.0.9", Port: 2222},
	}}

	w := credentialRequest(t, store, (*CredentialService).GetCredentials, http.MethodGet, "/credentials/",
		"", []string{adminPrivileges})

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "lab-root")
}

func TestCredentials_RotateCredential_ReportsAMissingRecordAs404(t *testing.T) {
	store := &fakeCredentialStore{available: true, rotateErr: credentials.ErrNotFound}

	w := credentialRequest(t, store, (*CredentialService).RotateCredential, http.MethodPut, "/credentials/12/secret",
		`{"secret":"new"}`, []string{adminPrivileges})

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "Credentials.NotFound", credentialErrorCode(t, w))
}

func TestCredentials_RotateCredential_HandsTheNewSecretToTheStore(t *testing.T) {
	store := &fakeCredentialStore{available: true}

	w := credentialRequest(t, store, (*CredentialService).RotateCredential, http.MethodPut, "/credentials/12/secret",
		`{"secret":"new-material"}`, []string{adminPrivileges})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "new-material", string(store.rotated[12]))
}

func TestCredentials_DeleteCredential_DeletesTheIdInThePath(t *testing.T) {
	store := &fakeCredentialStore{available: true}

	w := credentialRequest(t, store, (*CredentialService).DeleteCredential, http.MethodDelete, "/credentials/12",
		"", []string{adminPrivileges})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []int64{12}, store.deleted)
}
