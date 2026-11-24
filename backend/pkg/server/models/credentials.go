package models

import (
	"errors"
	"strings"
	"time"

	"opsagi/pkg/credentials"
)

// CredentialInfo is what the API shows about a stored secret. It has no field
// for the secret itself, so a handler cannot return one by accident.
type CredentialInfo struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Kind               string     `json:"kind"`
	Host               string     `json:"host"`
	Port               int        `json:"port"`
	Username           string     `json:"username"`
	HostKeyFingerprint string     `json:"host_key_fingerprint"`
	RotatedAt          time.Time  `json:"rotated_at"`
	LastUsedAt         *time.Time `json:"last_used_at"`
	CreatedAt          time.Time  `json:"created_at"`
}

// CredentialCreate is the write-only half: the secret arrives here and leaves as
// a sealed column, never coming back through the API.
type CredentialCreate struct {
	Name               string `json:"name" binding:"required"`
	Kind               string `json:"kind" binding:"required"`
	Secret             string `json:"secret" binding:"required"`
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Username           string `json:"username"`
	HostKeyFingerprint string `json:"host_key_fingerprint"`
}

func (c CredentialCreate) Valid() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(c.Secret) == "" {
		return errors.New("secret is required")
	}
	if c.Port < 0 || c.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	switch c.Kind {
	case credentials.KindSSHKey, credentials.KindSSHPassword, credentials.KindK8sToken, credentials.KindDBPassword:
		return nil
	}

	return errors.New("kind must be ssh_key, ssh_password, k8s_token or db_password")
}

// CredentialRotate carries a replacement secret.
type CredentialRotate struct {
	Secret string `json:"secret" binding:"required"`
}

func (c CredentialRotate) Valid() error {
	if strings.TrimSpace(c.Secret) == "" {
		return errors.New("secret is required")
	}

	return nil
}

// NewCredentialInfo drops the parts of a stored credential the API does not
// serve, which is everything but its metadata.
func NewCredentialInfo(c credentials.Credential) CredentialInfo {
	return CredentialInfo{
		ID:                 c.ID,
		Name:               c.Name,
		Kind:               c.Kind,
		Host:               c.Host,
		Port:               c.Port,
		Username:           c.Username,
		HostKeyFingerprint: c.HostKeyFingerprint,
		RotatedAt:          c.RotatedAt,
		LastUsedAt:         c.LastUsedAt,
		CreatedAt:          c.CreatedAt,
	}
}
