package credentials

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"opsagi/pkg/database"
)

// Kinds a credential can be. The remote tools only know how to use the two SSH
// forms today; the others are stored so a deployment can keep one inventory.
const (
	KindSSHKey      = "ssh_key"
	KindSSHPassword = "ssh_password"
	KindK8sToken    = "k8s_token"
	KindDBPassword  = "db_password"
)

var (
	// ErrNotFound is returned for a credential id or name that is not stored.
	ErrNotFound = errors.New("credential not found")
	// ErrBadKind is returned for a kind outside the stored ones.
	ErrBadKind = errors.New("unknown credential kind")
	// ErrBadName is returned for an empty or over-long name.
	ErrBadName = errors.New("credential name must be 1 to 128 characters")
)

// MaxNameLength bounds a name so a list stays readable.
const MaxNameLength = 128

// Credential is everything the API and the tools need about a stored secret. It
// has no field that can carry the secret itself.
type Credential struct {
	ID                 int64
	Name               string
	Kind               string
	Host               string
	Port               int
	Username           string
	HostKeyFingerprint string
	RotatedAt          time.Time
	LastUsedAt         *time.Time
	CreatedAt          time.Time
}

// Input describes a credential to create.
type Input struct {
	Name               string
	Kind               string
	Secret             []byte
	Host               string
	Port               int
	Username           string
	HostKeyFingerprint string
	CreatedBy          int64
}

// Queries is the slice of the generated query set the store needs, so a test
// can stand in for the database without implementing every query.
type Queries interface {
	CreateCredential(ctx context.Context, arg database.CreateCredentialParams) (database.Credential, error)
	GetCredential(ctx context.Context, id int64) (database.Credential, error)
	GetCredentialByName(ctx context.Context, name string) (database.Credential, error)
	ListCredentials(ctx context.Context) ([]database.ListCredentialsRow, error)
	RotateCredential(ctx context.Context, arg database.RotateCredentialParams) (database.RotateCredentialRow, error)
	TouchCredential(ctx context.Context, id int64) error
	DeleteCredential(ctx context.Context, id int64) error
}

// Store turns the plaintext an operator enters into the rows the tools read.
type Store struct {
	queries Queries
	sealer  *Sealer
}

// NewStore builds a store over the query set. A nil sealer is allowed: the store
// then reports ErrNoKey instead of writing a secret it cannot protect.
func NewStore(queries Queries, sealer *Sealer) *Store {
	return &Store{queries: queries, sealer: sealer}
}

// Available reports whether the store can seal and open secrets.
func (s *Store) Available() bool { return s != nil && s.queries != nil && s.sealer != nil }

// Create seals the secret and stores it with its metadata.
func (s *Store) Create(ctx context.Context, in Input) (Credential, error) {
	if !s.Available() {
		return Credential{}, ErrNoKey
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > MaxNameLength {
		return Credential{}, ErrBadName
	}
	if !knownKind(in.Kind) {
		return Credential{}, ErrBadKind
	}
	if len(in.Secret) == 0 {
		return Credential{}, errors.New("credential secret is empty")
	}

	sealed, err := s.sealer.Seal(in.Secret)
	if err != nil {
		return Credential{}, err
	}

	row, err := s.queries.CreateCredential(ctx, database.CreateCredentialParams{
		Name:               name,
		Kind:               in.Kind,
		Secret:             sealed,
		Host:               strings.TrimSpace(in.Host),
		Port:               int32(in.Port),
		Username:           strings.TrimSpace(in.Username),
		HostKeyFingerprint: strings.TrimSpace(in.HostKeyFingerprint),
		CreatedBy:          sql.NullInt64{Int64: in.CreatedBy, Valid: in.CreatedBy != 0},
	})
	if err != nil {
		return Credential{}, fmt.Errorf("store credential: %w", err)
	}

	return fromRow(row), nil
}

// List returns every credential without its secret.
func (s *Store) List(ctx context.Context) ([]Credential, error) {
	if s == nil || s.queries == nil {
		return nil, ErrNoKey
	}

	rows, err := s.queries.ListCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}

	out := make([]Credential, 0, len(rows))
	for _, row := range rows {
		out = append(out, Credential{
			ID:                 row.ID,
			Name:               row.Name,
			Kind:               row.Kind,
			Host:               row.Host,
			Port:               int(row.Port),
			Username:           row.Username,
			HostKeyFingerprint: row.HostKeyFingerprint,
			RotatedAt:          row.RotatedAt,
			LastUsedAt:         nullTime(row.LastUsedAt),
			CreatedAt:          row.CreatedAt,
		})
	}

	return out, nil
}

// Get returns one credential without its secret.
func (s *Store) Get(ctx context.Context, id int64) (Credential, error) {
	if s == nil || s.queries == nil {
		return Credential{}, ErrNoKey
	}

	row, err := s.queries.GetCredential(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Credential{}, ErrNotFound
		}
		return Credential{}, fmt.Errorf("read credential: %w", err)
	}

	return fromRow(row), nil
}

// Secret opens the stored secret. The caller must not log or persist the result.
func (s *Store) Secret(ctx context.Context, id int64) ([]byte, error) {
	if !s.Available() {
		return nil, ErrNoKey
	}

	row, err := s.queries.GetCredential(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("read credential: %w", err)
	}

	return s.sealer.Open(row.Secret)
}

// Rotate replaces the secret and moves rotated_at.
func (s *Store) Rotate(ctx context.Context, id int64, secret []byte) error {
	if !s.Available() {
		return ErrNoKey
	}
	if len(secret) == 0 {
		return errors.New("credential secret is empty")
	}

	sealed, err := s.sealer.Seal(secret)
	if err != nil {
		return err
	}

	if _, err := s.queries.RotateCredential(ctx, database.RotateCredentialParams{ID: id, Secret: sealed}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("rotate credential: %w", err)
	}

	return nil
}

// MarkUsed records that a tool dialled with this credential.
func (s *Store) MarkUsed(ctx context.Context, id int64) error {
	if s == nil || s.queries == nil {
		return ErrNoKey
	}

	return s.queries.TouchCredential(ctx, id)
}

// Delete removes the credential. Rows that still name it are not rewritten.
func (s *Store) Delete(ctx context.Context, id int64) error {
	if s == nil || s.queries == nil {
		return ErrNoKey
	}

	return s.queries.DeleteCredential(ctx, id)
}

func fromRow(row database.Credential) Credential {
	return Credential{
		ID:                 row.ID,
		Name:               row.Name,
		Kind:               row.Kind,
		Host:               row.Host,
		Port:               int(row.Port),
		Username:           row.Username,
		HostKeyFingerprint: row.HostKeyFingerprint,
		RotatedAt:          row.RotatedAt,
		LastUsedAt:         nullTime(row.LastUsedAt),
		CreatedAt:          row.CreatedAt,
	}
}

func nullTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}

	return &value.Time
}

func knownKind(kind string) bool {
	switch kind {
	case KindSSHKey, KindSSHPassword, KindK8sToken, KindDBPassword:
		return true
	}

	return false
}
