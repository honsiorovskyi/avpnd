package avpnd

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Runner runs the AWS VPN Client CLI and returns its stdout. The CLI reports
// failures as JSON ({"status":"Error","message":...}) with exit code 1, on
// stdout for some errors and stderr for others, so on failure the output is
// returned (stderr if stdout is empty) along with err.
type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// OSRunner runs the real CLI. Anything it prints to stderr goes to the logs.
//
// The CLI writes a few lines to ~/.config/AWSVPNClient/logs on every call and
// rotates at 1MB, so polling would bury the AWS client's real logs. The CLI
// finds that directory through $HOME, so it runs with HOME=home (the AWS
// daemon identifies the user by uid, so everything else still works) and the
// logs it left behind are deleted after each call.
type OSRunner struct {
	bin    string
	home   string
	logger *slog.Logger
	mu     sync.Mutex // calls are serialized so one call's cleanup can't hit another's log
}

// NewOSRunner creates a runner that uses home as the CLI's $HOME.
func NewOSRunner(bin, home string, logger *slog.Logger) (*OSRunner, error) {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, err
	}
	r := &OSRunner{bin: bin, home: home, logger: logger}
	r.removeLogs()
	return r, nil
}

func (r *OSRunner) removeLogs() {
	if err := os.RemoveAll(filepath.Join(r.home, ".config", "AWSVPNClient", "logs")); err != nil {
		r.logger.Warn("removing CLI logs", "error", err)
	}
}

func (r *OSRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.removeLogs()

	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Env = append(os.Environ(), "HOME="+r.home) // last value wins
	cmd.WaitDelay = 5 * time.Second                // don't hang on pipes held open by grandchildren
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if s := strings.TrimSpace(stderr.String()); s != "" {
		r.logger.Warn(s, "cmd", args[0], "stream", "stderr")
	}
	if err != nil && stdout.Len() == 0 {
		return stderr.Bytes(), err
	}
	return stdout.Bytes(), err
}
