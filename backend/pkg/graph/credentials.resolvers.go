package graph

import (
	"context"
	"errors"

	"opsagi/pkg/credentials"
	"opsagi/pkg/graph/model"
)

// credentialInfoFrom builds the GraphQL view of a stored credential. The secret
// has no field here and never leaves the store, so no resolver can leak it by
// returning what it read.
func credentialInfoFrom(c credentials.Credential) *model.CredentialInfo {
	return &model.CredentialInfo{
		ID:                 c.ID,
		Name:               c.Name,
		Kind:               model.CredentialKind(c.Kind),
		Host:               c.Host,
		Port:               c.Port,
		Username:           c.Username,
		HostKeyFingerprint: c.HostKeyFingerprint,
		RotatedAt:          c.RotatedAt,
		LastUsedAt:         c.LastUsedAt,
		CreatedAt:          c.CreatedAt,
	}
}

// derefString and derefInt read an optional input field as its zero value when
// the client left it out, which is how the REST DTO treats the same fields.
func derefString(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func derefInt(value *int) int {
	if value == nil {
		return 0
	}

	return *value
}

// Credentials is the resolver for the credentials field.
func (r *queryResolver) Credentials(ctx context.Context) ([]*model.CredentialInfo, error) {
	if _, _, err := validatePermission(ctx, "credentials.view"); err != nil {
		return nil, err
	}

	store := r.CredentialStore
	if store == nil || !store.Available() {
		return nil, errors.New("no credential master key is configured")
	}

	list, err := store.List(ctx)
	if err != nil {
		r.Logger.WithError(err).Error("list credentials")

		return nil, err
	}

	out := make([]*model.CredentialInfo, 0, len(list))
	for _, item := range list {
		out = append(out, credentialInfoFrom(item))
	}

	return out, nil
}

// CreateCredential is the resolver for the createCredential field.
func (r *mutationResolver) CreateCredential(ctx context.Context, input model.CredentialCreateInput) (*model.CredentialInfo, error) {
	uid, _, err := validatePermission(ctx, "credentials.manage")
	if err != nil {
		return nil, err
	}

	store := r.CredentialStore
	if store == nil || !store.Available() {
		return nil, errors.New("no credential master key is configured")
	}

	port := derefInt(input.Port)
	if port < 0 || port > 65535 {
		return nil, errors.New("port must be between 0 and 65535")
	}

	created, err := store.Create(ctx, credentials.Input{
		Name:               input.Name,
		Kind:               string(input.Kind),
		Secret:             []byte(input.Secret),
		Host:               derefString(input.Host),
		Port:               port,
		Username:           derefString(input.Username),
		HostKeyFingerprint: derefString(input.HostKeyFingerprint),
		CreatedBy:          uid,
	})
	if err != nil {
		r.Logger.WithError(err).Error("create credential")

		return nil, err
	}

	info := credentialInfoFrom(created)
	r.Subscriptions.NewCredentialPublisher(uid).CredentialAdded(ctx, info)

	return info, nil
}

// RotateCredential is the resolver for the rotateCredential field.
func (r *mutationResolver) RotateCredential(ctx context.Context, credentialID int64, input model.CredentialRotateInput) (*model.CredentialInfo, error) {
	uid, _, err := validatePermission(ctx, "credentials.manage")
	if err != nil {
		return nil, err
	}

	store := r.CredentialStore
	if store == nil || !store.Available() {
		return nil, errors.New("no credential master key is configured")
	}

	if err := store.Rotate(ctx, credentialID, []byte(input.Secret)); err != nil {
		r.Logger.WithError(err).Error("rotate credential")

		return nil, err
	}

	updated, err := store.Get(ctx, credentialID)
	if err != nil {
		r.Logger.WithError(err).Error("read credential after rotate")

		return nil, err
	}

	info := credentialInfoFrom(updated)
	r.Subscriptions.NewCredentialPublisher(uid).CredentialUpdated(ctx, info)

	return info, nil
}

// DeleteCredential is the resolver for the deleteCredential field.
func (r *mutationResolver) DeleteCredential(ctx context.Context, credentialID int64) (bool, error) {
	uid, _, err := validatePermission(ctx, "credentials.manage")
	if err != nil {
		return false, err
	}

	store := r.CredentialStore
	if store == nil || !store.Available() {
		return false, errors.New("no credential master key is configured")
	}

	if err := store.Delete(ctx, credentialID); err != nil {
		r.Logger.WithError(err).Error("delete credential")

		return false, err
	}

	r.Subscriptions.NewCredentialPublisher(uid).CredentialDeleted(ctx, &model.CredentialInfo{ID: credentialID})

	return true, nil
}

// CredentialAdded is the resolver for the credentialAdded field.
func (r *subscriptionResolver) CredentialAdded(ctx context.Context) (<-chan *model.CredentialInfo, error) {
	uid, _, err := validatePermission(ctx, "credentials.subscribe")
	if err != nil {
		return nil, err
	}

	return r.Subscriptions.NewCredentialSubscriber(uid).CredentialAdded(ctx)
}

// CredentialUpdated is the resolver for the credentialUpdated field.
func (r *subscriptionResolver) CredentialUpdated(ctx context.Context) (<-chan *model.CredentialInfo, error) {
	uid, _, err := validatePermission(ctx, "credentials.subscribe")
	if err != nil {
		return nil, err
	}

	return r.Subscriptions.NewCredentialSubscriber(uid).CredentialUpdated(ctx)
}

// CredentialDeleted is the resolver for the credentialDeleted field.
func (r *subscriptionResolver) CredentialDeleted(ctx context.Context) (<-chan *model.CredentialInfo, error) {
	uid, _, err := validatePermission(ctx, "credentials.subscribe")
	if err != nil {
		return nil, err
	}

	return r.Subscriptions.NewCredentialSubscriber(uid).CredentialDeleted(ctx)
}
