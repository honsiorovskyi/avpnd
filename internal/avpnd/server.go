// Package avpnd exposes the AWS VPN Client CLI over HTTP.
package avpnd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// samlCallbackPort is bound by the AWS VPN Client's own daemon for its SAML
// callback; avpnd must not take it.
const samlCallbackPort = 35001

const (
	statusTTL  = time.Second      // every CLI call also writes to the AWS client's log file
	cmdTimeout = 60 * time.Second // connect/disconnect return in ~1s; the daemon does the real work
)

// Server is a 1:1 HTTP front end for `aws-vpn-client` for a single profile.
type Server struct {
	profile string
	runner  Runner
	logger  *slog.Logger

	mu       sync.Mutex // held while the CLI runs, so concurrent /status calls share one call
	cached   *Status
	cachedAt time.Time
}

// Status is the /status response. Status is one of connected, connecting,
// disconnecting, disconnected; Raw is the CLI's own value.
type Status struct {
	Status string `json:"status"`
	Raw    string `json:"raw"`
}

// ValidateListen rejects addresses that would take the AWS client's SAML port.
func ValidateListen(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", addr, err)
	}
	if p, _ := strconv.Atoi(port); p == samlCallbackPort {
		return fmt.Errorf("port %d is reserved by the AWS VPN Client for its SAML callback; choose another -listen port", p)
	}
	return nil
}

func NewServer(profile string, runner Runner, logger *slog.Logger) *Server {
	return &Server{profile: profile, runner: runner, logger: logger}
}

// run executes a CLI command against the profile and logs its output at level. On
// failure it returns the CLI's own error message when it gave one.
func (s *Server) run(ctx context.Context, level slog.Level, cmd string) ([]byte, error) {
	out, err := s.runner.Run(ctx, cmd, "--profile-name", s.profile)
	s.logger.Log(ctx, level, "cli output", "cmd", cmd, "output", compact(out))
	if err == nil {
		return out, nil
	}
	var e struct{ Message string }
	if json.Unmarshal(out, &e) == nil && e.Message != "" {
		return out, errors.New(e.Message)
	}
	return out, err
}

func compact(b []byte) string {
	var buf bytes.Buffer
	if json.Compact(&buf, b) != nil {
		return strings.TrimSpace(string(b))
	}
	return buf.String()
}

// Status returns the profile's state, cached for statusTTL.
func (s *Server) Status(ctx context.Context) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cached != nil && time.Since(s.cachedAt) < statusTTL {
		return *s.cached, nil
	}
	out, err := s.run(ctx, slog.LevelDebug, "get-connection-status")
	if err != nil {
		return Status{}, err
	}
	var r struct {
		ConnectionStatus string `json:"connection-status"`
	}
	if err := json.Unmarshal(out, &r); err != nil || r.ConnectionStatus == "" {
		return Status{}, fmt.Errorf("unexpected get-connection-status output: %s", compact(out))
	}
	st := Status{Status: normalize(r.ConnectionStatus), Raw: r.ConnectionStatus}
	s.cached, s.cachedAt = &st, time.Now()
	return st, nil
}

// normalize maps the CLI's states (NotConnected, WaitingForIdentity,
// Connecting, Reconnecting, Connected, Disconnecting, Disconnected) to the
// four the extension understands.
func normalize(raw string) string {
	switch strings.ToLower(raw) {
	case "connected":
		return "connected"
	case "disconnecting":
		return "disconnecting"
	case "notconnected", "disconnected":
		return "disconnected"
	default: // connecting, waitingforidentity, reconnecting, anything new
		return "connecting"
	}
}

// do runs connect or disconnect and drops the status cache.
func (s *Server) do(ctx context.Context, cmd string) error {
	s.logger.Info(cmd, "profile", s.profile)
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	_, err := s.run(ctx, slog.LevelInfo, cmd)

	s.mu.Lock()
	s.cached = nil
	s.mu.Unlock()
	if err != nil {
		s.logger.Error(cmd+" failed", "profile", s.profile, "error", err)
	}
	return err
}

// endpoint adds CORS (Origin is echoed), OPTIONS preflight and a GET/POST check.
func endpoint(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		hdr := w.Header()
		hdr.Set("Access-Control-Allow-Origin", origin)
		hdr.Add("Vary", "Origin")

		switch r.Method {
		case http.MethodOptions:
			hdr.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			hdr.Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet, http.MethodPost:
			h(w, r)
		default:
			hdr.Set("Allow", "GET, POST, OPTIONS")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// command returns a handler that runs a CLI command: 204 on success, 502 with
// the CLI's message on failure. The command is not tied to the request, so a
// client hanging up can't kill it halfway.
func (s *Server) command(cmd string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.do(context.WithoutCancel(r.Context()), cmd); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), cmdTimeout)
	defer cancel()
	st, err := s.Status(ctx)
	if err != nil {
		s.logger.Error("status failed", "profile", s.profile, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/connect", endpoint(s.command("connect")))
	mux.HandleFunc("/disconnect", endpoint(s.command("disconnect")))
	mux.HandleFunc("/status", endpoint(s.handleStatus))
	return mux
}

// Serve listens on addr until SIGINT/SIGTERM. The VPN itself is left alone.
func (s *Server) Serve(addr string) error {
	if err := ValidateListen(addr); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.serve(ln)
}

func (s *Server) serve(ln net.Listener) error {
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	s.logger.Info("listening", "addr", ln.Addr().String(), "profile", s.profile)

	select {
	case err := <-errc:
		return err
	case <-sigCtx.Done():
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
