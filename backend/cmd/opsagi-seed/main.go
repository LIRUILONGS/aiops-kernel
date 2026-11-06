// Command opsagi-seed creates the first local administrator an installation
// needs before anyone can log in. There is no public sign-up, so without this
// the only way in is to write a bcrypt hash into the users table by hand.
//
//	opsagi-seed -mail ops@example.com -name "On Call"
//
// The password is read from -password, from OPSAGI_SEED_PASSWORD, or from stdin
// with -password-stdin (recommended: it keeps the secret out of the shell
// history and the process list). An existing user is left untouched unless
// -reset is passed.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"

	"opsagi/pkg/config"
	"opsagi/pkg/database"
	"opsagi/pkg/password"
)

const (
	roleAdmin = "admin"
	roleUser  = "user"
)

func main() {
	var (
		mail        = flag.String("mail", "admin@opsagi.com", "mail address to sign in with")
		name        = flag.String("name", "Administrator", "display name")
		role        = flag.String("role", roleAdmin, "role to grant: admin or user")
		passFlag    = flag.String("password", "", "password (or set OPSAGI_SEED_PASSWORD, or use -password-stdin)")
		passStdin   = flag.Bool("password-stdin", false, "read the password from stdin")
		reset       = flag.Bool("reset", false, "reset the password of an existing user instead of refusing")
		changeReq   = flag.Bool("password-change-required", false, "force the user to set a new password at first login")
		exitOnError = func(err error) {
			fmt.Fprintln(os.Stderr, "opsagi-seed: "+err.Error())
			os.Exit(1)
		}
	)
	flag.Parse()

	secret, err := readPassword(*passFlag, *passStdin, os.Stdin)
	if err != nil {
		exitOnError(err)
	}

	opts, err := buildOptions(*mail, *name, *role, secret, *reset, *changeReq)
	if err != nil {
		exitOnError(err)
	}

	cfg, err := config.NewConfig()
	if err != nil {
		exitOnError(fmt.Errorf("cannot read the configuration: %w", err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := database.EnsureTenantSchema(ctx, cfg); err != nil {
		exitOnError(fmt.Errorf("tenant schema initialization failed: %w", err))
	}

	boundedURL, err := database.WithStatementTimeout(cfg.DatabaseURL, database.StatementTimeout)
	if err != nil {
		exitOnError(fmt.Errorf("cannot bound database statements: %w", err))
	}

	db, err := sql.Open("postgres", boundedURL)
	if err != nil {
		exitOnError(fmt.Errorf("cannot open the database: %w", err))
	}
	defer db.Close()

	outcome, err := seedUser(ctx, db, opts)
	if err != nil {
		exitOnError(err)
	}

	fmt.Println(outcome)
}

type seedOptions struct {
	mail                   string
	name                   string
	role                   string
	passwordHash           string
	reset                  bool
	passwordChangeRequired bool
}

// readPassword resolves the secret from the three places it may come from and
// refuses an empty one, because an account with an empty hash cannot be logged
// into and the failure would only show up at the login screen.
func readPassword(fromFlag string, fromStdin bool, stdin io.Reader) (string, error) {
	secret := fromFlag
	if fromStdin && secret == "" {
		raw, err := io.ReadAll(io.LimitReader(stdin, 4096))
		if err != nil {
			return "", fmt.Errorf("cannot read the password from stdin: %w", err)
		}
		secret = strings.TrimRight(string(raw), "\r\n")
	}
	if secret == "" {
		secret = os.Getenv("OPSAGI_SEED_PASSWORD")
	}
	if secret == "" {
		return "", errors.New("no password given: use -password, -password-stdin or OPSAGI_SEED_PASSWORD")
	}

	return secret, nil
}

func buildOptions(mail, name, role, secret string, reset, changeRequired bool) (seedOptions, error) {
	opts := seedOptions{
		mail:                   strings.TrimSpace(mail),
		name:                   strings.TrimSpace(name),
		role:                   strings.ToLower(strings.TrimSpace(role)),
		reset:                  reset,
		passwordChangeRequired: changeRequired,
	}

	if opts.mail == "" || !strings.Contains(opts.mail, "@") {
		return seedOptions{}, fmt.Errorf("%q is not a mail address", mail)
	}
	if opts.name == "" {
		opts.name = opts.mail
	}
	if opts.role != roleAdmin && opts.role != roleUser {
		return seedOptions{}, fmt.Errorf("unknown role %q: use admin or user", role)
	}
	// The policy is the one the API enforces, so a seeded account can actually
	// log in instead of being handed a password the login form rejects.
	if !password.IsStrong(secret) {
		return seedOptions{}, errors.New("the password does not satisfy the policy: 16+ characters, or 8-15 with lower case, upper case, a digit and one of !@#$&*")
	}
	if !password.FitsHashLimit(secret) {
		return seedOptions{}, fmt.Errorf("the password is longer than the %d bytes bcrypt can hash", password.MaxBytes)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return seedOptions{}, fmt.Errorf("cannot hash the password: %w", err)
	}
	opts.passwordHash = string(hash)

	return opts, nil
}

// seedUser is the whole job, split out of main so the outcome text and the
// refusal paths can be checked without a process.
func seedUser(ctx context.Context, db *sql.DB, opts seedOptions) (string, error) {
	roleID, err := lookupRoleID(ctx, db, opts.role)
	if err != nil {
		return "", err
	}

	var existingID int64
	err = db.QueryRowContext(ctx, `SELECT id FROM users WHERE mail = $1`, opts.mail).Scan(&existingID)
	switch {
	case err == nil && !opts.reset:
		return "", fmt.Errorf("user %s already exists (id %d); pass -reset to set a new password", opts.mail, existingID)
	case err == nil:
		queries := database.New(db)
		if _, err := queries.UpdateUserPassword(ctx, database.UpdateUserPasswordParams{
			ID:       existingID,
			Password: sql.NullString{String: opts.passwordHash, Valid: true},
		}); err != nil {
			return "", fmt.Errorf("cannot reset the password of %s: %w", opts.mail, err)
		}
		if _, err := queries.UpdateUserRole(ctx, database.UpdateUserRoleParams{
			ID:     existingID,
			RoleID: roleID,
		}); err != nil {
			return "", fmt.Errorf("cannot set the role of %s: %w", opts.mail, err)
		}
		if _, err := queries.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
			ID:     existingID,
			Status: database.UserStatusActive,
		}); err != nil {
			return "", fmt.Errorf("cannot activate %s: %w", opts.mail, err)
		}
		if _, err := queries.UpdateUserPasswordChangeRequired(ctx, database.UpdateUserPasswordChangeRequiredParams{
			ID:                     existingID,
			PasswordChangeRequired: opts.passwordChangeRequired,
		}); err != nil {
			return "", fmt.Errorf("cannot record the password-change flag of %s: %w", opts.mail, err)
		}

		return fmt.Sprintf("reset the password of %s (id %d) and granted %s", opts.mail, existingID, opts.role), nil
	case errors.Is(err, sql.ErrNoRows):
		// handled below
	default:
		return "", fmt.Errorf("cannot look up %s: %w", opts.mail, err)
	}

	user, err := database.New(db).CreateUser(ctx, database.CreateUserParams{
		Type:                   database.UserTypeLocal,
		Mail:                   opts.mail,
		Name:                   opts.name,
		Password:               sql.NullString{String: opts.passwordHash, Valid: true},
		Status:                 database.UserStatusActive,
		RoleID:                 roleID,
		PasswordChangeRequired: opts.passwordChangeRequired,
	})
	if err != nil {
		return "", fmt.Errorf("cannot create %s: %w", opts.mail, err)
	}

	return fmt.Sprintf("created %s (id %d) with the %s role", user.Mail, user.ID, opts.role), nil
}

func lookupRoleID(ctx context.Context, db *sql.DB, role string) (int64, error) {
	name := "Admin"
	if role == roleUser {
		name = "User"
	}

	var id int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM roles WHERE name = $1`, name).Scan(&id); err != nil {
		return 0, fmt.Errorf("cannot find the %s role: %w (has the schema been migrated?)", name, err)
	}

	return id, nil
}
