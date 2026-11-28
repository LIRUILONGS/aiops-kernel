package services

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"opsagi/pkg/credentials"
	"opsagi/pkg/server/logger"
	"opsagi/pkg/server/models"
	"opsagi/pkg/server/response"

	"github.com/gin-gonic/gin"
)

// credentialStore is the part of credentials.Store the API needs.
type credentialStore interface {
	Available() bool
	List(ctx context.Context) ([]credentials.Credential, error)
	Create(ctx context.Context, in credentials.Input) (credentials.Credential, error)
	Rotate(ctx context.Context, id int64, secret []byte) error
	Delete(ctx context.Context, id int64) error
}

type CredentialService struct {
	store credentialStore
}

func NewCredentialService(store credentialStore) *CredentialService {
	return &CredentialService{store: store}
}

// GetCredentials is a function to return the stored credentials
// @Summary Retrieve stored credentials
// @Tags Credentials
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.successResp{data=[]models.CredentialInfo} "credential list received successful"
// @Failure 403 {object} response.errorResp "getting credentials not permitted"
// @Failure 503 {object} response.errorResp "no master key configured"
// @Router /credentials/ [get]
func (s *CredentialService) GetCredentials(c *gin.Context) {
	if !s.authorizeView(c) {
		return
	}
	if !s.store.Available() {
		response.Error(c, response.ErrCredentialsUnavailable, nil)

		return
	}

	list, err := s.store.List(c.Request.Context())
	if err != nil {
		logger.FromContext(c).Errorf("error listing credentials: %v", err)
		response.Error(c, response.ErrInternal, nil)

		return
	}

	out := make([]models.CredentialInfo, 0, len(list))
	for _, item := range list {
		out = append(out, models.NewCredentialInfo(item))
	}

	response.Success(c, http.StatusOK, out)
}

// CreateCredential is a function to store a new credential
// @Summary Store a credential
// @Tags Credentials
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body models.CredentialCreate true "credential data"
// @Success 200 {object} response.successResp{data=models.CredentialInfo} "credential stored successful"
// @Failure 403 {object} response.errorResp "storing credentials not permitted"
// @Failure 400 {object} response.errorResp "invalid credential data"
// @Failure 503 {object} response.errorResp "no master key configured"
// @Router /credentials/ [post]
func (s *CredentialService) CreateCredential(c *gin.Context) {
	if !s.authorizeManage(c) {
		return
	}
	if !s.store.Available() {
		response.Error(c, response.ErrCredentialsUnavailable, nil)

		return
	}

	var req models.CredentialCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.FromContext(c).Errorf("error binding credential data: %v", err)
		response.Error(c, response.ErrCredentialsInvalidRequest, err)

		return
	}
	if err := req.Valid(); err != nil {
		logger.FromContext(c).Errorf("error validating credential data: %v", err)
		response.Error(c, response.ErrCredentialsInvalidRequest, err)

		return
	}

	created, err := s.store.Create(c.Request.Context(), credentials.Input{
		Name:               req.Name,
		Kind:               req.Kind,
		Secret:             []byte(req.Secret),
		Host:               req.Host,
		Port:               req.Port,
		Username:           req.Username,
		HostKeyFingerprint: req.HostKeyFingerprint,
		CreatedBy:          int64(c.GetUint64("uid")),
	})
	if err != nil {
		s.reportWriteError(c, "storing", err)

		return
	}

	response.Success(c, http.StatusOK, models.NewCredentialInfo(created))
}

// RotateCredential is a function to replace a stored secret
// @Summary Replace a credential's secret
// @Tags Credentials
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "credential id"
// @Param request body models.CredentialRotate true "replacement secret"
// @Success 200 {object} response.successResp "credential rotated successful"
// @Failure 403 {object} response.errorResp "rotating credentials not permitted"
// @Failure 404 {object} response.errorResp "credential not found"
// @Failure 503 {object} response.errorResp "no master key configured"
// @Router /credentials/{id}/secret [put]
func (s *CredentialService) RotateCredential(c *gin.Context) {
	if !s.authorizeManage(c) {
		return
	}
	if !s.store.Available() {
		response.Error(c, response.ErrCredentialsUnavailable, nil)

		return
	}

	id, ok := credentialID(c)
	if !ok {
		return
	}

	var req models.CredentialRotate
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.FromContext(c).Errorf("error binding the replacement secret: %v", err)
		response.Error(c, response.ErrCredentialsInvalidRequest, err)

		return
	}
	if err := req.Valid(); err != nil {
		logger.FromContext(c).Errorf("error validating the replacement secret: %v", err)
		response.Error(c, response.ErrCredentialsInvalidRequest, err)

		return
	}

	if err := s.store.Rotate(c.Request.Context(), id, []byte(req.Secret)); err != nil {
		s.reportWriteError(c, "rotating", err)

		return
	}

	response.Success(c, http.StatusOK, nil)
}

// DeleteCredential is a function to remove a stored credential
// @Summary Delete a credential
// @Tags Credentials
// @Produce json
// @Security BearerAuth
// @Param id path int true "credential id"
// @Success 200 {object} response.successResp "credential deleted successful"
// @Failure 403 {object} response.errorResp "deleting credentials not permitted"
// @Router /credentials/{id} [delete]
func (s *CredentialService) DeleteCredential(c *gin.Context) {
	if !s.authorizeManage(c) {
		return
	}

	id, ok := credentialID(c)
	if !ok {
		return
	}

	if err := s.store.Delete(c.Request.Context(), id); err != nil {
		s.reportWriteError(c, "deleting", err)

		return
	}

	response.Success(c, http.StatusOK, nil)
}

// authorizeView gates the list. `credentials.admin` covers both halves, matching
// what `validatePermission` does on the GraphQL side: the two entries must agree
// on a role, or a grant passes one door and is refused at the other.
// `settings.admin` passes because it already means "this operator may change how
// the deployment works", and an instance installed before the credential area
// existed holds no `credentials.*` at all.
func (s *CredentialService) authorizeView(c *gin.Context) bool {
	return s.authorize(c, "credentials.view", "credentials.manage", "credentials.admin", "settings.admin")
}

// authorizeManage gates the writes. A caller holding only `credentials.view`
// reads the inventory and changes nothing.
func (s *CredentialService) authorizeManage(c *gin.Context) bool {
	return s.authorize(c, "credentials.manage", "credentials.admin", "settings.admin")
}

func (s *CredentialService) authorize(c *gin.Context, accepted ...string) bool {
	for _, priv := range c.GetStringSlice("prm") {
		for _, allowed := range accepted {
			if priv == allowed {
				return true
			}
		}
	}

	logger.FromContext(c).Errorf("error filtering user role permissions: trying %v", accepted)
	response.Error(c, response.ErrNotPermitted, nil)

	return false
}

func (s *CredentialService) reportWriteError(c *gin.Context, action string, err error) {
	switch {
	case errors.Is(err, credentials.ErrNotFound):
		response.Error(c, response.ErrCredentialsNotFound, err)
	case errors.Is(err, credentials.ErrNoKey):
		response.Error(c, response.ErrCredentialsUnavailable, err)
	case errors.Is(err, credentials.ErrBadKind), errors.Is(err, credentials.ErrBadName):
		response.Error(c, response.ErrCredentialsInvalidRequest, err)
	default:
		logger.FromContext(c).Errorf("error %s credential: %v", action, err)
		response.Error(c, response.ErrInternal, nil)
	}
}

func credentialID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		logger.FromContext(c).Errorf("error parsing credential id %q: %v", c.Param("id"), err)
		response.Error(c, response.ErrCredentialsInvalidRequest, err)

		return 0, false
	}

	return id, true
}
