// Copyright (C) 2026 honsiorovskyi
// SPDX-License-Identifier: AGPL-3.0-only

package avpnd

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeCLI = `#!/bin/sh
case "$1" in
  out) echo '{"a":1}';;
  err) echo '{"message":"boom"}' >&2; exit 1;;
  both) echo stdout-json; echo noise >&2; exit 1;;
  home)
    mkdir -p "$HOME/.config/AWSVPNClient/logs"
    echo log > "$HOME/.config/AWSVPNClient/logs/x.log"
    echo "$HOME";;
esac
`

func TestOSRunner(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "cli")
	home := filepath.Join(t.TempDir(), "home")
	if err := os.WriteFile(bin, []byte(fakeCLI), 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := NewOSRunner(bin, home, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if out, err := r.Run(ctx, "out"); err != nil || string(out) != "{\"a\":1}\n" {
		t.Errorf("out: %q %v", out, err)
	}
	// On failure the CLI's JSON may be on stderr (used when stdout is empty).
	if out, err := r.Run(ctx, "err"); err == nil || string(out) != "{\"message\":\"boom\"}\n" {
		t.Errorf("err: %q %v", out, err)
	}
	if out, err := r.Run(ctx, "both"); err == nil || string(out) != "stdout-json\n" {
		t.Errorf("both: %q %v", out, err)
	}

	// The CLI runs with HOME=home and the logs it wrote there are gone afterwards.
	out, err := r.Run(ctx, "home")
	if err != nil || strings.TrimSpace(string(out)) != home {
		t.Errorf("home: %q %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "AWSVPNClient", "logs")); !os.IsNotExist(err) {
		t.Errorf("CLI logs were not removed: %v", err)
	}
}
