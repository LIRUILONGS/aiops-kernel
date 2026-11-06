package main

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestSeed_ReadPassword_ResolvesTheThreeSources(t *testing.T) {
	tests := []struct {
		name        string
		flag        string
		stdin       bool
		stdinText   string
		env         string
		want        string
		wantRefusal string
	}{
		{
			name: "the flag wins over everything",
			flag: "Flag-Password-2026!",
			env:  "Env-Password-2026!",
			want: "Flag-Password-2026!",
		},
		{
			name:      "stdin is used when the flag is empty",
			stdin:     true,
			stdinText: "Stdin-Password-2026!\r\n",
			env:       "Env-Password-2026!",
			want:      "Stdin-Password-2026!",
		},
		{
			name: "the environment is the fallback",
			env:  "Env-Password-2026!",
			want: "Env-Password-2026!",
		},
		{
			name:        "nothing anywhere is a refusal, not an empty password",
			wantRefusal: "no password given",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OPSAGI_SEED_PASSWORD", tc.env)

			got, err := readPassword(tc.flag, tc.stdin, strings.NewReader(tc.stdinText))

			if tc.wantRefusal != "" {
				if err == nil {
					t.Fatalf("accepted no password at all")
				}
				if !strings.Contains(err.Error(), tc.wantRefusal) {
					t.Errorf("refusal %q does not say %q", err, tc.wantRefusal)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused a password it was given: %v", err)
			}
			if got != tc.want {
				t.Errorf("password = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSeed_BuildOptions_RefusesWhatTheLoginFormWouldReject(t *testing.T) {
	const strong = "Seed-Check-2026!"

	tests := []struct {
		name        string
		mail        string
		role        string
		password    string
		wantRefusal string
		wantName    string
	}{
		{
			name:     "an administrator with a strong password",
			mail:     "ops@example.com",
			role:     roleAdmin,
			password: strong,
			wantName: "ops@example.com",
		},
		{
			name:     "the name is kept when given",
			mail:     "ops@example.com",
			role:     roleUser,
			password: strong,
			wantName: "ops@example.com",
		},
		{
			name:        "a mail address without an at sign",
			mail:        "ops",
			role:        roleAdmin,
			password:    strong,
			wantRefusal: "is not a mail address",
		},
		{
			name:        "a role that does not exist",
			mail:        "ops@example.com",
			role:        "root",
			password:    strong,
			wantRefusal: "unknown role",
		},
		{
			name:        "a password the policy rejects",
			mail:        "ops@example.com",
			role:        roleAdmin,
			password:    "short",
			wantRefusal: "does not satisfy the policy",
		},
		{
			name:        "a password bcrypt cannot hash",
			mail:        "ops@example.com",
			role:        roleAdmin,
			password:    strings.Repeat("a", 73),
			wantRefusal: "longer than the 72 bytes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := buildOptions(tc.mail, "", tc.role, tc.password, false, false)

			if tc.wantRefusal != "" {
				if err == nil {
					t.Fatalf("accepted %q, which the API would reject", tc.mail+" "+tc.password)
				}
				if !strings.Contains(err.Error(), tc.wantRefusal) {
					t.Errorf("refusal %q does not say %q", err, tc.wantRefusal)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused a valid account: %v", err)
			}
			if opts.name != tc.wantName {
				t.Errorf("name = %q, want %q", opts.name, tc.wantName)
			}
			if err := bcrypt.CompareHashAndPassword([]byte(opts.passwordHash), []byte(tc.password)); err != nil {
				t.Errorf("the stored hash does not open with the password it was built from: %v", err)
			}
		})
	}
}
