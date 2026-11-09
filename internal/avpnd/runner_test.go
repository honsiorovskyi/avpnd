package avpnd

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestOSRunnerStreams(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "cli")
	script := "#!/bin/sh\ncase \"$1\" in\n out) echo '{\"a\":1}';;\n err) echo '{\"message\":\"boom\"}' >&2; exit 1;;\n both) echo stdout-json; echo noise >&2; exit 1;;\nesac\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewOSRunner(bin, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	if out, err := r.Run(ctx, "out"); err != nil || string(out) != "{\"a\":1}\n" {
		t.Errorf("out: %q %v", out, err)
	}
	if out, err := r.Run(ctx, "err"); err == nil || string(out) != "{\"message\":\"boom\"}\n" {
		t.Errorf("err: %q %v", out, err)
	}
	if out, err := r.Run(ctx, "both"); err == nil || string(out) != "stdout-json\n" {
		t.Errorf("both: %q %v", out, err)
	}
}
