// Package localai adapts a running LocalAI OpenAI-compatible endpoint to the
// native, identity-bound inference leaf. LocalAI retains lifecycle authority
// for its backend processes, model loading, scheduling and eviction. OA
// attaches only to a LocalAI v4.10.0 process that meets the conditions of
// research/security-reviews/localai-4.10.0.md; LaunchRecipe produces a launch
// that meets them.
package localai

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	identity "github.com/openabstractions/abstraction-identity"
	inference "github.com/openabstractions/abstraction-inference/go"
	"github.com/openabstractions/abstraction-provider-kit/go/enginehttp"
	"github.com/openabstractions/abstraction-provider-kit/go/providerhost"
	router "github.com/openabstractions/abstraction-router/go"
)

const ProviderName = "localai"

// ListenerIdentity is the process generation and executable path observed
// from the loopback listener. Verify Program with the operator before pinning
// Process in Config.
type ListenerIdentity struct {
	Process      identity.Process
	ProcessProof identity.Proof
	Program      string
	ProgramProof identity.Proof
}

// InspectListener captures the exact process generation listening on a LocalAI
// loopback endpoint without sending HTTP bytes. The caller should inspect the
// returned PID and Program, then pass Process unchanged to Config.ExpectedProcess.
// The native HTTP client rechecks that generation for every new connection.
func InspectListener(base string) (ListenerIdentity, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	peer, err := enginehttp.InspectListener(ctx, base)
	if err != nil {
		return ListenerIdentity{}, fmt.Errorf("localai: read listener identity: %w", err)
	}
	process, processProof := peer.Process.Get()
	program, programProof := peer.Path.Get()
	if processProof < identity.ProofBound || process.PID <= 0 || process.StartTime.IsZero() || process.Recycled {
		return ListenerIdentity{}, errors.New("localai: listener process generation is not kernel-bound")
	}
	return ListenerIdentity{Process: process, ProcessProof: processProof, Program: program, ProgramProof: programProof}, nil
}

// Config describes an already-running LocalAI instance. ExpectedProcess is an
// operator-selected listener identity from the same machine. The native HTTP
// client verifies its PID and process start time before sending request bytes
// and while receiving the response. Attach mode has no authority to stop or
// unload LocalAI or its backend children.
type Config struct {
	Endpoint        string
	RuntimePrograms []string
	BaseURL         string
	Models          []string
	ExpectedProcess identity.Process
	// APIKey is the per-launch key LocalAI was started with in
	// LOCALAI_API_KEY (see LaunchRecipe). OA sends it with every request.
	APIKey string
	// ModelFiles pins each GGUF file a model configuration names, by file
	// name in the models directory, to its lowercase hex SHA-256.
	ModelFiles map[string]string
	Inference  inference.Config
}

// New composes a loopback OpenAI-compatible route and serves it through the
// shared designated leaf. It attaches only to a listener process that meets
// every condition of the LocalAI v4.10.0 security review and otherwise
// returns a *RefusalError naming the unmet condition. Config cannot
// substitute an unverified HTTP client or a remote router.
func New(cfg Config) (*providerhost.Host, error) {
	if cfg.Endpoint == "" || len(cfg.RuntimePrograms) == 0 {
		return nil, errors.New("localai: native endpoint and designated runtime required")
	}
	if len(cfg.Models) == 0 {
		return nil, errors.New("localai: explicit model allowlist required")
	}
	if cfg.ExpectedProcess.PID <= 0 || cfg.ExpectedProcess.StartTime.IsZero() || cfg.ExpectedProcess.Recycled {
		return nil, errors.New("localai: operator-selected listener process generation required")
	}
	if cfg.Inference.Decide == nil || cfg.Inference.Apply == nil {
		return nil, errors.New("localai: inference decision and credential applier required")
	}
	if !apiKeyShape.MatchString(cfg.APIKey) {
		return nil, refuse(ConditionAPIKey, "OA has no per-launch API key to send")
	}
	if len(cfg.ModelFiles) == 0 {
		return nil, refuse(ConditionLocalModels, "no GGUF file is pinned")
	}
	if err := checkListener(cfg.BaseURL, cfg.ExpectedProcess); err != nil {
		return nil, err
	}
	u, _ := url.Parse(cfg.BaseURL)
	view, err := observeProcess(cfg.ExpectedProcess.PID)
	if err != nil {
		return nil, err
	}
	want := attachWant{Port: u.Port(), APIKey: cfg.APIKey, Models: cfg.Models, ModelFiles: cfg.ModelFiles}
	if err := verifyAttach(view, want, attachPins); err != nil {
		return nil, err
	}
	// The PID read above belongs to the selected generation only while that
	// generation still holds the listener afterwards.
	if err := checkListener(cfg.BaseURL, cfg.ExpectedProcess); err != nil {
		return nil, err
	}

	client, err := enginehttp.NewClient(cfg.BaseURL, cfg.ExpectedProcess)
	if err != nil {
		return nil, err
	}
	cfg.Inference.Router = router.New(router.OpenAICompatible(ProviderName, cfg.BaseURL, "/v1", cfg.Models))
	cfg.Inference.HTTP = withAPIKey(client, cfg.APIKey)
	return providerhost.New(providerhost.Config{
		Endpoint:        cfg.Endpoint,
		RuntimePrograms: cfg.RuntimePrograms,
		Inference:       cfg.Inference,
	})
}

// attachPins is the reviewed artifact set New requires.
var attachPins = reviewedV4100

func checkListener(base string, expected identity.Process) error {
	observed, err := InspectListener(base)
	if err != nil {
		return err
	}
	if observed.Process.PID != expected.PID || !observed.Process.StartTime.Equal(expected.StartTime) {
		return errors.New("localai: configured listener process generation is stale or changed")
	}
	return nil
}
