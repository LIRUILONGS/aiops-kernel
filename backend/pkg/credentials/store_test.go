package credentials

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"opsagi/pkg/database"
)

type fakeQueries struct {
	rows map[int64]database.Credential

	created   []database.CreateCredentialParams
	rotated   []database.RotateCredentialParams
	touched   []int64
	deleted   []int64
	lastList  []database.ListCredentialsRow
	createErr error
}

func newFakeQueries() *fakeQueries {
	return &fakeQueries{rows: make(map[int64]database.Credential)}
}

func (f *fakeQueries) CreateCredential(_ context.Context, arg database.CreateCredentialParams) (database.Credential, error) {
	if f.createErr != nil {
		return database.Credential{}, f.createErr
	}
	f.created = append(f.created, arg)

	id := int64(len(f.rows) + 1)
	row := database.Credential{
		ID:                 id,
		Name:               arg.Name,
		Kind:               arg.Kind,
		Secret:             arg.Secret,
		Host:               arg.Host,
		Port:               arg.Port,
		Username:           arg.Username,
		HostKeyFingerprint: arg.HostKeyFingerprint,
		RotatedAt:          time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC),
		CreatedBy:          arg.CreatedBy,
		CreatedAt:          time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC),
	}
	f.rows[id] = row

	return row, nil
}

func (f *fakeQueries) GetCredential(_ context.Context, id int64) (database.Credential, error) {
	row, ok := f.rows[id]
	if !ok {
		return database.Credential{}, sql.ErrNoRows
	}

	return row, nil
}

func (f *fakeQueries) GetCredentialByName(_ context.Context, name string) (database.Credential, error) {
	for _, row := range f.rows {
		if row.Name == name {
			return row, nil
		}
	}

	return database.Credential{}, sql.ErrNoRows
}

func (f *fakeQueries) ListCredentials(_ context.Context) ([]database.ListCredentialsRow, error) {
	out := make([]database.ListCredentialsRow, 0, len(f.rows))
	for _, row := range f.rows {
		out = append(out, database.ListCredentialsRow{
			ID:                 row.ID,
			Name:               row.Name,
			Kind:               row.Kind,
			Host:               row.Host,
			Port:               row.Port,
			Username:           row.Username,
			HostKeyFingerprint: row.HostKeyFingerprint,
			RotatedAt:          row.RotatedAt,
			LastUsedAt:         row.LastUsedAt,
			CreatedBy:          row.CreatedBy,
			CreatedAt:          row.CreatedAt,
		})
	}
	f.lastList = out

	return out, nil
}

func (f *fakeQueries) RotateCredential(_ context.Context, arg database.RotateCredentialParams) (database.RotateCredentialRow, error) {
	row, ok := f.rows[arg.ID]
	if !ok {
		return database.RotateCredentialRow{}, sql.ErrNoRows
	}
	f.rotated = append(f.rotated, arg)

	row.Secret = arg.Secret
	row.RotatedAt = row.RotatedAt.Add(time.Hour)
	f.rows[arg.ID] = row

	return database.RotateCredentialRow{ID: arg.ID, RotatedAt: row.RotatedAt}, nil
}

func (f *fakeQueries) TouchCredential(_ context.Context, id int64) error {
	f.touched = append(f.touched, id)

	return nil
}

func (f *fakeQueries) DeleteCredential(_ context.Context, id int64) error {
	f.deleted = append(f.deleted, id)

	return nil
}

func newStore(t *testing.T) (*Store, *fakeQueries) {
	t.Helper()

	sealer, err := NewSealer(newTestKey(t))
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}

	fake := newFakeQueries()

	return NewStore(fake, sealer), fake
}

func TestCredentials_Store_CreateSealsTheSecretBeforeTheDatabase(t *testing.T) {
	store, fake := newStore(t)
	plaintext := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nlive one\n")

	created, err := store.Create(context.Background(), Input{
		Name:               "lab-root",
		Kind:               KindSSHKey,
		Secret:             plaintext,
		Host:               "10.0.0.9",
		Port:               22,
		Username:           "root",
		HostKeyFingerprint: "SHA256:abc",
		CreatedBy:          7,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if created.Name != "lab-root" || created.Port != 22 || created.HostKeyFingerprint != "SHA256:abc" {
		t.Fatalf("created = %+v, want the metadata it was given", created)
	}
	if len(fake.created) != 1 {
		t.Fatalf("the database saw %d inserts, want 1", len(fake.created))
	}
	if bytes.Contains(fake.created[0].Secret, plaintext) {
		t.Fatal("the database received the plaintext")
	}

	opened, err := store.Secret(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("read the secret back: %v", err)
	}
	if !bytes.Equal(opened, plaintext) {
		t.Fatalf("secret = %q, want %q", opened, plaintext)
	}
}

func TestCredentials_Store_CreateRefusesWhatCannotBeStored(t *testing.T) {
	store, _ := newStore(t)
	sealed := Input{Name: "x", Kind: KindSSHKey, Secret: []byte("s")}

	tests := []struct {
		name  string
		input Input
		want  error
	}{
		{
			name:  "empty name",
			input: Input{Name: "  ", Kind: KindSSHKey, Secret: []byte("s")},
			want:  ErrBadName,
		},
		{
			name:  "name longer than the limit",
			input: Input{Name: string(make([]byte, MaxNameLength+1)), Kind: KindSSHKey, Secret: []byte("s")},
			want:  ErrBadName,
		},
		{
			name:  "unknown kind",
			input: Input{Name: "x", Kind: "kerberos", Secret: []byte("s")},
			want:  ErrBadKind,
		},
		{
			name:  "empty secret",
			input: Input{Name: "x", Kind: KindSSHKey},
			want:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.Create(context.Background(), tc.input)
			if err == nil {
				t.Fatal("the store accepted it")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}

	if len(sealed.Secret) == 0 {
		t.Fatal("the fixture lost its secret")
	}
}

func TestCredentials_Store_WithoutASealerRefusesToStore(t *testing.T) {
	store := NewStore(newFakeQueries(), nil)

	if store.Available() {
		t.Fatal("a store without a sealer reports itself available")
	}
	if _, err := store.Create(context.Background(), Input{Name: "x", Kind: KindSSHKey, Secret: []byte("s")}); !errors.Is(err, ErrNoKey) {
		t.Fatalf("create = %v, want ErrNoKey", err)
	}
	if _, err := store.Secret(context.Background(), 1); !errors.Is(err, ErrNoKey) {
		t.Fatalf("secret = %v, want ErrNoKey", err)
	}
}

func TestCredentials_Store_ListShowsTheMetadataOnly(t *testing.T) {
	store, fake := newStore(t)

	if _, err := store.Create(context.Background(), Input{
		Name: "lab-root", Kind: KindSSHKey, Secret: []byte("private"), Host: "10.0.0.9", Port: 22, Username: "root",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	used := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	row := fake.rows[1]
	row.LastUsedAt = sql.NullTime{Time: used, Valid: true}
	fake.rows[1] = row

	list, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list returned %d rows, want 1", len(list))
	}
	if list[0].LastUsedAt == nil || !list[0].LastUsedAt.Equal(used) {
		t.Fatalf("last_used_at = %v, want %v", list[0].LastUsedAt, used)
	}
	if len(fake.lastList) != 1 {
		t.Fatalf("the query returned %d rows, want 1", len(fake.lastList))
	}
}

func TestCredentials_Store_GetAndSecretSayWhenTheRowIsGone(t *testing.T) {
	store, _ := newStore(t)

	if _, err := store.Get(context.Background(), 42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get = %v, want ErrNotFound", err)
	}
	if _, err := store.Secret(context.Background(), 42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("secret = %v, want ErrNotFound", err)
	}
	if err := store.Rotate(context.Background(), 42, []byte("new")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rotate = %v, want ErrNotFound", err)
	}
}

func TestCredentials_Store_RotateReplacesTheSecretAndMovesTheClock(t *testing.T) {
	store, fake := newStore(t)

	created, err := store.Create(context.Background(), Input{Name: "lab-root", Kind: KindSSHKey, Secret: []byte("old")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := store.Rotate(context.Background(), created.ID, []byte("new")); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	opened, err := store.Secret(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("read the secret: %v", err)
	}
	if string(opened) != "new" {
		t.Fatalf("secret = %q, want the rotated one", opened)
	}
	if len(fake.rotated) != 1 {
		t.Fatalf("the database saw %d rotations, want 1", len(fake.rotated))
	}
	if bytes.Contains(fake.rotated[0].Secret, []byte("new")) {
		t.Fatal("the rotated secret reached the database in the clear")
	}
}

func TestCredentials_Store_MarkUsedAndDeleteReachTheQueries(t *testing.T) {
	store, fake := newStore(t)

	if err := store.MarkUsed(context.Background(), 5); err != nil {
		t.Fatalf("mark used: %v", err)
	}
	if err := store.Delete(context.Background(), 5); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if len(fake.touched) != 1 || fake.touched[0] != 5 {
		t.Fatalf("touched = %v, want [5]", fake.touched)
	}
	if len(fake.deleted) != 1 || fake.deleted[0] != 5 {
		t.Fatalf("deleted = %v, want [5]", fake.deleted)
	}
}
