package localai

import (
	"context"
	"errors"
	"fmt"
	"github.com/openabstractions/abstraction-identity/go/privatedir/privatetest"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	identity "github.com/openabstractions/abstraction-identity"
	inference "github.com/openabstractions/abstraction-inference/go"
	"github.com/openabstractions/abstraction-provider-kit/go/enginehttp"
)

func baseConfig(t *testing.T) Config {
	t.Helper()
	endpoint := filepath.Join(privatetest.Dir(t), "localai.sock")
	if runtime.GOOS == "windows" {
		endpoint = fmt.Sprintf(`\\.\pipe\localai-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	selected, err := InspectListener(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newAttachFixture(t, u.Port())
	useFixtureProcess(t, fixture, selected.Process.PID)
	return Config{
		Endpoint:        endpoint,
		RuntimePrograms: []string{os.Args[0]},
		BaseURL:         server.URL,
		Models:          []string{"qwen3-0.6b-q4_0"},
		ExpectedProcess: selected.Process,
		APIKey:          fixture.key,
		ModelFiles:      fixture.want.ModelFiles,
		Inference: inference.Config{
			Decide: func(context.Context, inference.Subject, string, string) (string, error) { return "permitted", nil },
			Apply: func(context.Context, inference.Subject, string, string, string) (map[string]string, string) {
				return nil, "applied"
			},
		},
	}
}

// useFixtureProcess makes New observe the fixture installation in place of
// the listener process (the test binary) and require the fixture's pins.
func useFixtureProcess(t *testing.T, f *attachFixture, pid int) {
	t.Helper()
	observe, pins := observeProcess, attachPins
	t.Cleanup(func() { observeProcess, attachPins = observe, pins })
	observeProcess = func(got int) (processView, error) {
		if got != pid {
			return processView{}, fmt.Errorf("observed pid %d, want %d", got, pid)
		}
		return f.view, nil
	}
	attachPins = f.pins
}

func requireLoopbackIdentity(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "darwin" {
		t.Skip("identity does not bind loopback listener processes on macOS")
	}
}

func TestNewRequiresExplicitLocalIdentityAndModel(t *testing.T) {
	requireLoopbackIdentity(t)
	base := baseConfig(t)
	cases := []struct {
		name string
		edit func(*Config)
	}{
		{"runtime", func(c *Config) { c.RuntimePrograms = nil }},
		{"model", func(c *Config) { c.Models = nil }},
		{"process", func(c *Config) { c.ExpectedProcess = identity.Process{} }},
		{"recycled-process", func(c *Config) { c.ExpectedProcess.Recycled = true }},
		{"stale-process-generation", func(c *Config) { c.ExpectedProcess.StartTime = c.ExpectedProcess.StartTime.Add(time.Second) }},
		{"decision", func(c *Config) { c.Inference.Decide = nil }},
		{"applier", func(c *Config) { c.Inference.Apply = nil }},
		{"non-loopback", func(c *Config) { c.BaseURL = "https://example.test" }},
		{"api-key", func(c *Config) { c.APIKey = "" }},
		{"short-api-key", func(c *Config) { c.APIKey = "secret" }},
		{"other-api-key", func(c *Config) { c.APIKey = strings.Repeat("c", 64) }},
		{"model-pins", func(c *Config) { c.ModelFiles = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.edit(&cfg)
			if host, err := New(cfg); err == nil {
				_ = host.Close()
				t.Fatal("invalid config accepted")
			}
		})
	}
}

func TestNewServesDesignatedLocalLeaf(t *testing.T) {
	requireLoopbackIdentity(t)
	host, err := New(baseConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestNewRefusesAListenerThatIsNotReviewedLocalAI uses the real process
// observer on the test binary: Linux reads it and refuses its program hash;
// other platforms cannot read it and refuse observation.
func TestNewRefusesAListenerThatIsNotReviewedLocalAI(t *testing.T) {
	requireLoopbackIdentity(t)
	cfg := baseConfig(t)
	observeProcess, attachPins = procObserve, reviewedV4100
	host, err := New(cfg)
	if err == nil {
		_ = host.Close()
		t.Fatal("test binary accepted as LocalAI")
	}
	var refusal *RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("New error = %v, want a refusal", err)
	}
	want := ConditionProcessObservable
	if runtime.GOOS == "linux" {
		want = ConditionProgramPin
	}
	if refusal.Condition != want {
		t.Fatalf("condition = %s (%s), want %s", refusal.Condition, refusal.Detail, want)
	}
}

func TestNewRefusesAnUnsafeFixtureBeforeServing(t *testing.T) {
	requireLoopbackIdentity(t)
	cfg := baseConfig(t)
	// baseConfig installed the fixture view; make it autoload galleries.
	view, err := observeProcess(cfg.ExpectedProcess.PID)
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	for _, a := range view.Args {
		if a != "--autoload-galleries=false" {
			args = append(args, a)
		}
	}
	view.Args = args
	observeProcess = func(int) (processView, error) { return view, nil }
	var refusal *RefusalError
	if host, err := New(cfg); !errors.As(err, &refusal) || refusal.Condition != ConditionGalleriesOff {
		if host != nil {
			_ = host.Close()
		}
		t.Fatalf("New = %v, want galleries-off refusal", err)
	}
}

func TestInspectListenerPinsTheObservedProcessGeneration(t *testing.T) {
	requireLoopbackIdentity(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	selected, err := InspectListener(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Process.PID != os.Getpid() || selected.ProcessProof < identity.ProofBound || selected.Process.StartTime.IsZero() {
		t.Fatalf("unexpected listener identity: %+v", selected)
	}
	accepted := make(chan net.Conn, 1)
	// This direct identity control uses a separate listener: the immediate
	// binder intentionally does not wait for an HTTP server's Accept.
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() { c, _ := listener.Accept(); accepted <- c }()
	conn, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	serverConn := <-accepted
	if serverConn == nil {
		t.Fatal("direct identity control did not accept")
	}
	defer serverConn.Close()
	binding, err := identity.BindLoopbackServer(conn, &identity.Options{ConnectedAt: time.Now(), SkipCodeSignature: true})
	if err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	peer, err := binding.Peer()
	if err != nil {
		_ = binding.Close()
		_ = conn.Close()
		t.Fatal(err)
	}
	actual, proof := peer.Process.Get()
	t.Logf("selected=%+v actual=%+v proof=%s", selected.Process, actual, proof)
	_ = binding.Close()
	_ = conn.Close()
	client, err := enginehttp.NewClient(server.URL, selected.Process)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", response.StatusCode)
	}

	stale := selected.Process
	stale.StartTime = stale.StartTime.Add(time.Second)
	client, err = enginehttp.NewClient(server.URL, stale)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(server.URL + "/health"); err == nil {
		t.Fatal("stale process generation reached listener")
	}
}

func TestNewRefusesWhenLoopbackProcessProofIsUnavailable(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("this platform can bind loopback listener processes")
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	cfg := Config{
		Endpoint:        "unused",
		RuntimePrograms: []string{os.Args[0]},
		BaseURL:         server.URL,
		Models:          []string{"fixture"},
		ExpectedProcess: identity.Process{PID: os.Getpid(), StartTime: time.Now()},
		APIKey:          strings.Repeat("k", 64),
		ModelFiles:      map[string]string{"fixture.gguf": strings.Repeat("0", 64)},
		Inference: inference.Config{
			Decide: func(context.Context, inference.Subject, string, string) (string, error) { return "permitted", nil },
			Apply: func(context.Context, inference.Subject, string, string, string) (map[string]string, string) {
				return nil, "applied"
			},
		},
	}
	if _, err := New(cfg); !errors.Is(err, identity.ErrNoBinding) {
		t.Fatalf("New error = %v, want loopback identity refusal", err)
	}
}
