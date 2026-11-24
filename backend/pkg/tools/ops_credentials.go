package tools

import (
	"context"
	"errors"
	"fmt"

	"opsagi/pkg/credentials"
	"opsagi/pkg/database"
	"opsagi/pkg/opsremote"

	"github.com/sirupsen/logrus"
)

// credentialSource is the part of the credential store the tools need. A store
// built from a deployment without a master key answers Available false, and a
// target that names a credential then fails with that sentence rather than a
// dial error two layers away.
type credentialSource interface {
	Available() bool
	Get(ctx context.Context, id int64) (credentials.Credential, error)
	Secret(ctx context.Context, id int64) ([]byte, error)
	MarkUsed(ctx context.Context, id int64) error
}

// credentialStore returns the store the ops tools dial through, or nil when this
// deployment has no master key. A key that is set but malformed is reported
// once, loudly: a target that names a credential would otherwise fail its dial
// with "no authentication method", which reads as a configuration mistake two
// layers away from the real one.
func credentialStore(queries database.Querier) *credentials.Store {
	sealer, err := credentials.NewSealerFromEnv()
	if err != nil {
		if errors.Is(err, credentials.ErrNoKey) {
			return nil
		}
		logrus.WithError(err).Error(credentials.VarKey + " is set but is not a usable master key; stored credentials are not reachable")

		return nil
	}

	return credentials.NewStore(queries, sealer)
}

// WithCredentialStore attaches the store the targets reference. Without one, a
// target that names a credential is reported as unreachable rather than dialled
// with no authentication at all.
func (o *opsTools) WithCredentialStore(store credentialSource) *opsTools {
	o.credentials = store

	return o
}

// materialize turns a configured target into one Dial can use: when it names a
// credential, the secret is opened here, in memory, and never written to disk.
// The credential's host, user and pin fill anything the target left out, so an
// operator can write `{"id":"lab","credential_id":3}` and be understood.
func (o *opsTools) materialize(ctx context.Context, target opsremote.Target) (opsremote.Target, error) {
	if target.CredentialID == 0 {
		return target, nil
	}
	if o.credentials == nil || !o.credentials.Available() {
		return target, fmt.Errorf("目标 %q 引用了凭据 %d，但这个部署没有可用的凭据主密钥（%s）；请配置后重试", target.ID, target.CredentialID, credentials.VarKey)
	}

	record, err := o.credentials.Get(ctx, target.CredentialID)
	if err != nil {
		return target, fmt.Errorf("读取凭据 %d 失败：%w", target.CredentialID, err)
	}

	secret, err := o.credentials.Secret(ctx, target.CredentialID)
	if err != nil {
		return target, fmt.Errorf("打开凭据 %d 失败：%w", target.CredentialID, err)
	}

	switch record.Kind {
	case credentials.KindSSHPassword:
		target.Password = string(secret)
	case credentials.KindSSHKey:
		target.Secret = secret
	default:
		return target, fmt.Errorf("凭据 %d 的类型是 %q，不能用来登录 SSH", target.CredentialID, record.Kind)
	}

	if target.Host == "" {
		target.Host = record.Host
	}
	if target.Port == 0 {
		target.Port = record.Port
	}
	if target.User == "" {
		target.User = record.Username
	}
	if target.HostKeyFingerprint == "" {
		target.HostKeyFingerprint = record.HostKeyFingerprint
	}

	return target, nil
}
