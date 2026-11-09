// Copyright (C) 2026 honsiorovskyi
// SPDX-License-Identifier: AGPL-3.0-only

package avpnd

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRunner struct {
	mu    sync.Mutex
	calls []string
	out   map[string]string // first arg -> stdout
	fail  map[string]bool
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, strings.Join(args, " "))
	var err error
	if f.fail[args[0]] {
		err = errors.New("exit status 1")
	}
	return []byte(f.out[args[0]]), err
}

func (f *fakeRunner) count(cmd string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, cmd+" ") {
			n++
		}
	}
	return n
}

func statusJSON(s string) string {
	return `{"connection-status":"` + s + `","latest-connection-attempt":{"initiated-by":"x"}}`
}

func newTest(f *fakeRunner) *Server {
	return NewServer("P", f, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func get(h http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestStatusMapping(t *testing.T) {
	for raw, want := range map[string]string{
		"Connected":          "connected",
		"NotConnected":       "disconnected",
		"Disconnected":       "disconnected",
		"Disconnecting":      "disconnecting",
		"Connecting":         "connecting",
		"WaitingForIdentity": "connecting",
		"Reconnecting":       "connecting",
		"SomethingNew":       "connecting",
	} {
		s := newTest(&fakeRunner{out: map[string]string{"get-connection-status": statusJSON(raw)}})
		st, err := s.Status(context.Background())
		if err != nil || st.Status != want || st.Raw != raw {
			t.Errorf("%s: got %+v, %v; want %s", raw, st, err, want)
		}
	}
}

func TestStatusUsesProfileAndCaches(t *testing.T) {
	f := &fakeRunner{out: map[string]string{"get-connection-status": statusJSON("Connected")}}
	s := newTest(f)
	for i := 0; i < 5; i++ {
		if _, err := s.Status(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count("get-connection-status"); n != 1 {
		t.Fatalf("expected 1 CLI call within TTL, got %d", n)
	}
	if f.calls[0] != "get-connection-status --profile-name P" {
		t.Fatalf("unexpected args %q", f.calls[0])
	}
	s.cachedAt = time.Now().Add(-2 * statusTTL)
	s.Status(context.Background())
	if n := f.count("get-connection-status"); n != 2 {
		t.Fatalf("expected refresh after TTL, got %d calls", n)
	}
}

func TestConcurrentStatusSharesOneCall(t *testing.T) {
	f := &fakeRunner{out: map[string]string{"get-connection-status": statusJSON("Connected")}}
	s := newTest(f)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.Status(context.Background()) }()
	}
	wg.Wait()
	if n := f.count("get-connection-status"); n != 1 {
		t.Fatalf("expected 1 CLI call, got %d", n)
	}
}

func TestConnectDisconnectRunCLIAndInvalidateCache(t *testing.T) {
	f := &fakeRunner{out: map[string]string{
		"get-connection-status": statusJSON("NotConnected"),
		"connect":               `{"status":"WaitingForIdentity"}`,
	}}
	s := newTest(f)
	h := s.Handler()
	get(h, "GET", "/status")

	if w := get(h, "GET", "/connect"); w.Code != http.StatusNoContent {
		t.Fatalf("connect: %d", w.Code)
	}
	if f.count("connect") != 1 || f.calls[1] != "connect --profile-name P" {
		t.Fatalf("calls: %v", f.calls)
	}
	get(h, "GET", "/status")
	if f.count("get-connection-status") != 2 {
		t.Fatal("connect should invalidate the status cache")
	}
	if w := get(h, "POST", "/disconnect"); w.Code != http.StatusNoContent {
		t.Fatalf("disconnect: %d", w.Code)
	}
	if f.count("disconnect") != 1 {
		t.Fatalf("calls: %v", f.calls)
	}
}

func TestCLIErrorMessageIsPassedThrough(t *testing.T) {
	f := &fakeRunner{
		out:  map[string]string{"connect": `{"status":"Error","message":"Already connected to profile"}`},
		fail: map[string]bool{"connect": true},
	}
	w := get(newTest(f).Handler(), "GET", "/connect")
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "Already connected to profile") {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
}

func TestStatusError(t *testing.T) {
	f := &fakeRunner{
		out:  map[string]string{"get-connection-status": `{"status":"Error","message":"Profile not found"}`},
		fail: map[string]bool{"get-connection-status": true},
	}
	w := get(newTest(f).Handler(), "GET", "/status")
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "Profile not found") {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
	if _, err := newTest(&fakeRunner{out: map[string]string{"get-connection-status": "garbage"}}).Status(context.Background()); err == nil {
		t.Fatal("expected error for unparseable output")
	}
}

func TestHTTP(t *testing.T) {
	f := &fakeRunner{out: map[string]string{"get-connection-status": statusJSON("Connected")}}
	h := newTest(f).Handler()

	for _, path := range []string{"/status", "/connect", "/disconnect"} {
		w := get(h, "OPTIONS", path)
		if w.Code != http.StatusNoContent || w.Header().Get("Access-Control-Allow-Origin") != "http://example.com" {
			t.Errorf("%s preflight: %d %v", path, w.Code, w.Header())
		}
		if w := get(h, "PUT", path); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s PUT: %d", path, w.Code)
		}
	}
	if w := get(h, "GET", "/shutdown"); w.Code != http.StatusNotFound {
		t.Errorf("/shutdown should not exist, got %d", w.Code)
	}
	w := get(h, "GET", "/status")
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" ||
		strings.TrimSpace(w.Body.String()) != `{"status":"connected","raw":"Connected"}` {
		t.Errorf("status: %d %q", w.Code, w.Body.String())
	}
}

func TestValidateListen(t *testing.T) {
	for addr, ok := range map[string]bool{
		"127.0.0.1:35002": true, "127.0.0.1:35001": false, "localhost:35001": false, ":35001": false, "nonsense": false,
	} {
		if err := ValidateListen(addr); (err == nil) != ok {
			t.Errorf("%s: err=%v", addr, err)
		}
	}
}

func TestServeRefusesReservedPort(t *testing.T) {
	if err := newTest(&fakeRunner{}).Serve("127.0.0.1:35001"); err == nil {
		t.Fatal("expected error")
	}
}

func TestServeOverRealListener(t *testing.T) {
	f := &fakeRunner{out: map[string]string{"get-connection-status": statusJSON("Connected")}}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go newTest(f).serve(ln)
	resp, err := http.Get("http://" + ln.Addr().String() + "/status")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %v", resp, err)
	}
	resp.Body.Close()
}
