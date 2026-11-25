package graph

import (
	"context"

	"opsagi/pkg/config"
	"opsagi/pkg/controller"
	"opsagi/pkg/credentials"
	"opsagi/pkg/database"
	"opsagi/pkg/database/knowledge"
	"opsagi/pkg/graph/subscriptions"
	"opsagi/pkg/providers"
	"opsagi/pkg/server/auth"
	"opsagi/pkg/server/update"
	"opsagi/pkg/templates"
	"opsagi/pkg/timezone"

	"github.com/sirupsen/logrus"
	"github.com/vxcontrol/cloud/anonymizer"
)

// CredentialStore is the slice of the credential store the GraphQL layer needs.
// It mirrors the tools' credentialSource without the MarkUsed method, because
// the management UI creates, rotates and deletes credentials but never dials.
type CredentialStore interface {
	Available() bool
	List(ctx context.Context) ([]credentials.Credential, error)
	Get(ctx context.Context, id int64) (credentials.Credential, error)
	Create(ctx context.Context, in credentials.Input) (credentials.Credential, error)
	Rotate(ctx context.Context, id int64, secret []byte) error
	Delete(ctx context.Context, id int64) error
}

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct {
	DB              database.Querier
	Config          *config.Config
	Logger          *logrus.Entry
	TokenCache      *auth.TokenCache
	DefaultPrompter templates.Prompter
	ProvidersCtrl   providers.ProviderController
	Controller      controller.FlowController
	Subscriptions   subscriptions.SubscriptionsController
	Knowledge       knowledge.KnowledgeStore
	Replacer        anonymizer.Replacer
	Updates         *update.Service
	Timezones       *timezone.Catalog
	CredentialStore CredentialStore
}
