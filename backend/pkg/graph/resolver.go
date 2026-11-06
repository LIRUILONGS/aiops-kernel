package graph

import (
	"opsagi/pkg/config"
	"opsagi/pkg/controller"
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
}
