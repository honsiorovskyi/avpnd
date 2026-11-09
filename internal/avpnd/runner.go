package avpnd

import (
	"bytes"
	"context"
	"log/slog"
	"os/exec"
	"strings"
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
type OSRunner struct {
	bin    string
	logger *slog.Logger
}

func NewOSRunner(bin string, logger *slog.Logger) *OSRunner {
	return &OSRunner{bin: bin, logger: logger}
}

func (r *OSRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.WaitDelay = 5 * time.Second // don't hang on pipes held open by grandchildren
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
