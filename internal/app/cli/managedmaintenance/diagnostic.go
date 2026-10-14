package managedmaintenance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

const diagnosticLimit = 64 << 10
const diagnosticPrefix = "leapview-managed-failure-v1 "

// diagnosticTail is private, bounded process output. Only fixed classifications
// derived from it leave memory; neither this buffer nor the command is logged.
type diagnosticTail struct {
	mu   sync.Mutex
	data []byte
}

func (b *diagnosticTail) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if n >= diagnosticLimit {
		b.data = append(b.data[:0], p[n-diagnosticLimit:]...)
		return n, nil
	}
	if excess := len(b.data) + n - diagnosticLimit; excess > 0 {
		copy(b.data, b.data[excess:])
		b.data = b.data[:len(b.data)-excess]
	}
	b.data = append(b.data, p...)
	return n, nil
}

type commandFailure struct {
	command, reason string
	exitCode        int
	err             error
}

func (e *commandFailure) Error() string {
	return fmt.Sprintf("managed %s command failed: %v", e.command, e.err)
}
func (e *commandFailure) Unwrap() error { return e.err }

type ingressFailure struct {
	stage string
	err   error
}

func (e *ingressFailure) Error() string { return fmt.Sprintf("managed ingress %s: %v", e.stage, e.err) }
func (e *ingressFailure) Unwrap() error { return e.err }

func classifyCommandFailure(output []byte) string {
	text := strings.ToLower(string(output))
	for _, rule := range []struct{ needle, reason string }{
		{"net::ssh::authenticationfailed", "ssh-authentication"},
		{"net::ssh::hostkeymismatch", "ssh-host-key"},
		{"net::ssh::hostkeyunknown", "ssh-host-key"},
		{"net::ssh::connectiontimeout", "ssh-connection"},
		{"connection refused", "connection-refused"},
		{"bundler::gemnotfound", "bundler-dependency"},
		{"could not find gem", "bundler-dependency"},
		{"cannot load such file", "ruby-load"},
		{"cannot connect to the docker daemon", "docker-daemon"},
		{"no such image", "docker-image"},
		{"network kamal not found", "docker-network"},
		{"container name \"/kamal-proxy\" is already in use", "docker-container"},
		{"managed controller lock identity mismatch", "controller-lock"},
		{"managed controller lock is held by another owner", "controller-lock"},
		{"managed mutation requires a controller lock", "controller-lock"},
		{"managed mutation requires a live bounded deadline", "controller-deadline"},
		{"managed mutation requires an isolated process group", "controller-process-group"},
	} {
		if strings.Contains(text, rule.needle) {
			return rule.reason
		}
	}
	return "unknown"
}

const managedFailureNonceEnv = "LEAPVIEW_MANAGED_FAILURE_NONCE"
const rubyFailurePrefix = "leapview-ruby-failure-v1 "

var rubyFailureReasons = map[string]struct{}{
	"ssh-authentication": {}, "ssh-host-key": {}, "ssh-connection": {},
	"connection-refused": {}, "connection-timeout": {}, "connection-unreachable": {},
	"remote-command": {}, "kamal-configuration": {}, "kamal-hook": {}, "kamal-lock": {},
	"ruby-argument": {}, "local-file": {},
}

func newManagedFailureNonce() string {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(nonce[:])
}

func failureNonceFromEnv(env []string) string {
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if name != managedFailureNonceEnv {
			continue
		}
		if ok && len(value) == 32 {
			if _, err := hex.DecodeString(value); err == nil && strings.ToLower(value) == value {
				return value
			}
		}
		return ""
	}
	return ""
}

// classifyRubyFailure accepts only a single exact marker bound to this local
// command's unexported nonce. Remote command output cannot fabricate a useful
// class category without knowing the nonce, and no exception text is parsed.
func classifyRubyFailure(output []byte, nonce string) string {
	if len(nonce) != 32 {
		return ""
	}
	if _, err := hex.DecodeString(nonce); err != nil || strings.ToLower(nonce) != nonce {
		return ""
	}
	prefix := rubyFailurePrefix + nonce + " "
	category := ""
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		if category != "" {
			return ""
		}
		candidate := strings.TrimSuffix(line, "\r")
		candidate = strings.TrimPrefix(candidate, prefix)
		if _, ok := rubyFailureReasons[candidate]; !ok {
			return ""
		}
		category = candidate
	}
	return category
}

func newCommandFailure(ctx context.Context, bin string, err error, output []byte) error {
	return newCommandFailureWithNonce(ctx, bin, err, output, "")
}

func newCommandFailureWithNonce(ctx context.Context, bin string, err error, output []byte, nonce string) error {
	name := "other"
	if bin == "bundle" || bin == "docker" {
		name = bin
	}
	reason, code := classifyCommandFailure(output), -1
	if rubyReason := classifyRubyFailure(output, nonce); rubyReason != "" {
		reason = rubyReason
	}
	var exited *exec.ExitError
	if errors.As(err, &exited) {
		code = exited.ExitCode()
	} else {
		reason = "start-failed"
	}
	if ctx.Err() != nil {
		reason = "canceled"
	}
	return &commandFailure{command: name, reason: reason, exitCode: code, err: err}
}

// A separate marker avoids parsing human diagnostics or accepting arbitrary
// child output as structured evidence. The original error remains unchanged.
func writeFailureDiagnostic(w io.Writer, err error) {
	if err == nil {
		return
	}
	var ingress *ingressFailure
	var child *commandFailure
	hasIngress, hasChild := errors.As(err, &ingress), errors.As(err, &child)
	if !hasIngress && !hasChild {
		return
	}
	value := struct {
		Version  int    `json:"schemaVersion"`
		Stage    string `json:"stage"`
		Command  string `json:"command"`
		Reason   string `json:"reason"`
		ExitCode int    `json:"exitCode"`
	}{1, "command", "none", "unknown", -1}
	if hasIngress {
		value.Stage = ingress.stage
		// A joined cleanup error can contain a different ingress boundary.
		// Never attribute the first child failure to that later boundary.
		var nested *commandFailure
		if hasChild && (!errors.As(ingress.err, &nested) || nested != child) {
			value.Stage = "command"
		}
	}
	if hasChild {
		value.Command, value.Reason, value.ExitCode = child.command, child.reason, child.exitCode
	}
	data, e := json.Marshal(value)
	if e == nil {
		_, _ = fmt.Fprintf(w, "%s%s\n", diagnosticPrefix, data)
	}
}
