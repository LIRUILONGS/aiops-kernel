// Package opsremote runs read-only diagnostics against a remote host over SSH.
//
// Every command is built from a fixed template rather than assembled from user
// input, so a probe cannot be turned into an arbitrary shell. The only place
// free-form input is allowed is Target.Command, which callers must validate
// against Allowlist before use.
package opsremote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// DefaultTimeout bounds a single remote command. Probes that scan many
// containers run several commands, each with its own deadline.
const DefaultTimeout = 20 * time.Second

// MaxOutputBytes truncates captured output. A runaway `journalctl` or `dmesg`
// can otherwise pull hundreds of megabytes into the agent's context.
const MaxOutputBytes = 256 * 1024

// Target describes how to reach a host. Password and KeyFile are alternatives;
// when both are empty the agent is used, which only works for key-based setups.
type Target struct {
	ID       string `json:"id"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password,omitempty"`
	KeyFile  string `json:"key_file,omitempty"`

	// HostKeyFingerprint pins the key the host presents, as ssh-keygen -lf
	// prints it (SHA256:... or the MD5: form of an older key). Empty means the
	// key is accepted as it comes, which is only ever right for a host reached
	// over a channel that already authenticates it.
	HostKeyFingerprint string `json:"host_key_fingerprint,omitempty"`
}

// Result is the outcome of one remote command.
type Result struct {
	Command   string        `json:"command"`
	Stdout    string        `json:"stdout"`
	Stderr    string        `json:"stderr"`
	ExitCode  int           `json:"exit_code"`
	Duration  time.Duration `json:"duration_ns"`
	Truncated bool          `json:"truncated"`
}

// Client is a single SSH connection. It is safe for concurrent use: ssh.Client
// multiplexes channels, and each call opens its own session.
type Client struct {
	target  Target
	client  *ssh.Client
	timeout time.Duration

	// runStdin is the seam the trace retry is tested through; a nil value means
	// the real SSH session.
	runStdin func(ctx context.Context, command, stdin string) (Result, error)
}

// Dial connects to the target. The caller owns the returned client and must
// close it.
func Dial(ctx context.Context, target Target) (*Client, error) {
	if target.Host == "" {
		return nil, errors.New("target host is required")
	}
	if target.Port == 0 {
		target.Port = 22
	}
	if target.User == "" {
		target.User = "root"
	}

	auths := make([]ssh.AuthMethod, 0, 2)
	if target.Password != "" {
		auths = append(auths, ssh.Password(target.Password))
	}
	if target.KeyFile != "" {
		key, err := os.ReadFile(target.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse key file: %w", err)
		}
		auths = append(auths, ssh.PublicKeys(signer))
	}
	if len(auths) == 0 {
		return nil, errors.New("no authentication method: set password or key_file")
	}

	cfg := &ssh.ClientConfig{
		User:            target.User,
		Auth:            auths,
		HostKeyCallback: hostKeyCallback(target),
		Timeout:         DefaultTimeout,
	}

	addr := net.JoinHostPort(target.Host, fmt.Sprintf("%d", target.Port))

	dialer := &net.Dialer{Timeout: DefaultTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}

	// ssh.NewClientConn has no context, so the handshake is bounded by the
	// connection deadline rather than by ctx.
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(DefaultTimeout))
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh handshake: %w", err)
	}
	_ = conn.SetDeadline(time.Time{})

	return &Client{
		target:  target,
		client:  ssh.NewClient(sshConn, chans, reqs),
		timeout: DefaultTimeout,
	}, nil
}

// hostKeyCallback enforces the pin on a target that carries one. A target
// without a pin keeps the behaviour it had before pinning existed: the key is
// taken as presented. Operators get the pinned form from `ssh-keygen -lf` on
// the host, and the credential record carries it once the store is in use.
func hostKeyCallback(target Target) ssh.HostKeyCallback {
	pinned := strings.TrimSpace(target.HostKeyFingerprint)
	if pinned == "" {
		return ssh.InsecureIgnoreHostKey()
	}

	return func(hostname string, _ net.Addr, key ssh.PublicKey) error {
		if fingerprintMatches(pinned, key) {
			return nil
		}

		return fmt.Errorf("host key mismatch for %s: the pin says %s, the host presented %s", hostname, pinned, ssh.FingerprintSHA256(key))
	}
}

// fingerprintMatches accepts the forms ssh-keygen prints, with or without the
// algorithm prefix and the base64 padding. An MD5 fingerprint carries no prefix
// in the current form, so its sixteen colon-separated bytes identify it.
func fingerprintMatches(pinned string, key ssh.PublicKey) bool {
	pin := strings.TrimSpace(pinned)
	if strings.HasPrefix(strings.ToUpper(pin), "MD5:") || strings.Count(pin, ":") == 15 {
		return strings.EqualFold(trimFingerprint(pin, "MD5:"), trimFingerprint(ssh.FingerprintLegacyMD5(key), "MD5:"))
	}

	return trimFingerprint(pin, "SHA256:") == trimFingerprint(ssh.FingerprintSHA256(key), "SHA256:")
}

func trimFingerprint(value, prefix string) string {
	if len(value) >= len(prefix) && strings.EqualFold(value[:len(prefix)], prefix) {
		value = value[len(prefix):]
	}

	return strings.TrimRight(value, "=")
}

// Close releases the connection.
func (c *Client) Close() error {
	if c.client == nil {
		return nil
	}
	return c.client.Close()
}

// Target returns the target the client was dialled for.
func (c *Client) Target() Target { return c.target }

// Run executes one command and captures its output. A non-zero exit status is
// returned in Result.ExitCode rather than as an error: "no failed units" and
// "systemctl is not installed" are both answers, and the caller decides which
// matters.
func (c *Client) Run(ctx context.Context, command string) (Result, error) {
	started := time.Now()
	res := Result{Command: command}

	session, err := c.client.NewSession()
	if err != nil {
		return res, fmt.Errorf("open session: %w", err)
	}
	defer func() { _ = session.Close() }()

	var stdout, stderr bytes.Buffer
	session.Stdout = &limitedWriter{buf: &stdout, remaining: MaxOutputBytes}
	session.Stderr = &limitedWriter{buf: &stderr, remaining: MaxOutputBytes / 8}

	done := make(chan error, 1)
	go func() { done <- session.Run(command) }()

	select {
	case err = <-done:
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
		// What the command managed to print before it hung is usually the reason
		// it hung, so a timeout keeps it rather than throwing the evidence away.
		res.Stdout = stdout.String()
		res.Stderr = stderr.String()
		res.Duration = time.Since(started)
		res.Truncated = stdout.Len() >= MaxOutputBytes
		return res, fmt.Errorf("command timed out: %w", ctx.Err())
	}

	res.Stdout = stdout.String()
	res.Stderr = stderr.String()
	res.Duration = time.Since(started)
	res.Truncated = stdout.Len() >= MaxOutputBytes

	var exitErr *ssh.ExitError
	switch {
	case err == nil:
		res.ExitCode = 0
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitStatus()
	default:
		return res, fmt.Errorf("run command: %w", err)
	}

	return res, nil
}

// RunStdin executes one command with stdin attached, for the callers that pass
// data rather than only a command line: the sandbox ships the agent's script this
// way so it never becomes part of the ssh command string. A non-zero exit status
// lands in Result.ExitCode, as in Run.
func (c *Client) RunStdin(ctx context.Context, command, stdin string) (Result, error) {
	started := time.Now()
	res := Result{Command: command}

	session, err := c.client.NewSession()
	if err != nil {
		return res, fmt.Errorf("open session: %w", err)
	}
	defer func() { _ = session.Close() }()

	var stdout, stderr bytes.Buffer
	session.Stdin = strings.NewReader(stdin)
	session.Stdout = &limitedWriter{buf: &stdout, remaining: MaxOutputBytes}
	session.Stderr = &limitedWriter{buf: &stderr, remaining: MaxOutputBytes / 8}

	done := make(chan error, 1)
	go func() { done <- session.Run(command) }()

	select {
	case err = <-done:
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
		// What the command managed to print before it hung is usually the reason
		// it hung, so a timeout keeps it rather than throwing the evidence away.
		res.Stdout = stdout.String()
		res.Stderr = stderr.String()
		res.Duration = time.Since(started)
		res.Truncated = stdout.Len() >= MaxOutputBytes
		return res, fmt.Errorf("command timed out: %w", ctx.Err())
	}

	res.Stdout = stdout.String()
	res.Stderr = stderr.String()
	res.Duration = time.Since(started)
	res.Truncated = stdout.Len() >= MaxOutputBytes

	var exitErr *ssh.ExitError
	switch {
	case err == nil:
		res.ExitCode = 0
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitStatus()
	default:
		return res, fmt.Errorf("run command: %w", err)
	}

	return res, nil
}

// RunCtx runs a command with the default timeout.
func (c *Client) RunCtx(parent context.Context, command string) (Result, error) {
	ctx, cancel := context.WithTimeout(parent, c.timeout)
	defer cancel()
	return c.Run(ctx, command)
}

type limitedWriter struct {
	buf       *bytes.Buffer
	remaining int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return len(p), nil
	}
	if len(p) > w.remaining {
		p = p[:w.remaining]
	}
	n, err := w.buf.Write(p)
	w.remaining -= n
	return len(p), err
}

// RedactPassword strips the target password from any text that may be echoed
// back - a command line, an error, or a log excerpt.
func RedactPassword(text string, target Target) string {
	if target.Password == "" {
		return text
	}
	return strings.ReplaceAll(text, target.Password, "***")
}
