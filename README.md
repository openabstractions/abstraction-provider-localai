# abstraction-provider-localai

*For operators and provider authors writing Go.*

This Go package lets OA applications chat with models on a LocalAI server
(a program that runs models on this machine) that is already running.

**What's new here:** A LocalAI server joins OA only after the adapter has read
its executable, command line and environment and matched them to a reviewed
configuration.

This package exposes that server through OA's native `abstraction.inference/chat@1` contract. It uses LocalAI's OpenAI-compatible `/v1` API, an explicit model allowlist, and OA's shared designated provider leaf. The adapter fixes execution to local placement, verifies the loopback listener's process identity using the configured PID and start time, and attaches only to a LocalAI process configured as OA's LocalAI v4.10.0 security review requires (see [Status](#status)).

## Launch

`LaunchRecipe` returns the command line, complete environment and working directory for LocalAI v4.10.0 that the attach check accepts. The adapter does not start LocalAI; the operator or a launcher runs the recipe exactly, with no inherited environment.

```go
key, err := localai.NewAPIKey()
if err != nil { return err }
launch, err := localai.LaunchRecipe(localai.LaunchPaths{
    Program: "/srv/localai/local-ai",   // local-ai-v4.10.0-linux-amd64
    Dir:     "/srv/localai",
    Port:    18080,
    APIKey:  key,
})
```

The recipe binds `127.0.0.1:<port>`, passes the key in `LOCALAI_API_KEY`, sets `--galleries=[]`, `--backend-galleries=[]`, `--autoload-galleries=false`, `--autoload-backend-galleries=false`, `--disable-gallery-endpoint` and `--auto-upgrade-backends=false`, and points every LocalAI path, `HOME` and the working directory inside `Dir`. Before launch, `Dir/models` holds one YAML configuration per model and its GGUF file, and `Dir/backends/cpu-llama-cpp` holds the reviewed backend. Install the backend by its pinned OCI digest, `local-ai backends install quay.io/go-skynet/local-ai-backends@sha256:9d6b85ce9a0463e715a9cff7cbf4e8327182e44203e9f926988972e340cfda74`, with `--backends-path` set to `Dir/backends`. That install writes a `metadata.json` holding exactly `name`, `installed_at`, `uri` and `digest`, the four fields the check accepts. A gallery install also records `gallery_url` and `version` (core/gallery/backends.go:428-434) and is refused.

## Attach

```go
host, err := localai.New(localai.Config{
    Endpoint:        endpoint,
    RuntimePrograms: []string{runtimeExecutable},
    BaseURL:         "http://127.0.0.1:18080",
    Models:          []string{"qwen3-0.6b-q4_0"},
    ExpectedProcess: selectedLocalAIListener,
    APIKey:          key,
    ModelFiles:      map[string]string{"Qwen3-0.6B-Q4_0.gguf": "33bcc570…e8f75cb"},
    Inference: inference.Config{
        Decide: rights.Decide,
        Apply:  credentials.Apply,
    },
})
var refusal *localai.RefusalError
if errors.As(err, &refusal) {
    log.Printf("LocalAI not attached: %s: %s", refusal.Condition, refusal.Detail)
}
```

`New` reads the listener process's executable, command line, environment and working directory from `/proc`, then checks each condition and returns a `*RefusalError` (matching `localai.ErrRefused`) that names the first unmet one:

| Condition | Required |
| --- | --- |
| `process-observable` | Linux, the same mount namespace as OA, and a process OA may read (same user or `CAP_SYS_PTRACE`); paths resolve in the process's root |
| `program-pin` | executable SHA-256 `412cde3d…be8c006` (LocalAI v4.10.0 linux-amd64) |
| `loopback-listener` | address `127.0.0.1:<BaseURL port>` from every source; no socket activation, `--mitm-listen` or `LOCALAI_ALLOW_INSECURE_PUBLIC_BIND` |
| `api-key` | `LOCALAI_API_KEY` equal to `Config.APIKey`; no `--api-keys` on the command line, no `api_keys.json` |
| `galleries-off` | empty model and backend galleries, no autoload, no backend auto-upgrade, no preloads or external backends |
| `gallery-endpoint-off` | `--disable-gallery-endpoint` |
| `no-p2p-federation` | no P2P, P2P token, federation or distributed mode |
| `no-remote-backends` | no `LLAMACPP_GRPC_SERVERS`, external gRPC backends or `external_backends.json`; no model option `grpc_servers`, `rpc_servers` or `--rpc` |
| `clean-environment` | no `HF_*`, proxy, `LD_*` or `BASH_ENV` variables, no LocalAI dotenv files, no download credentials, no model `env:` block |
| `backend-manifest` | the backends path holds only `cpu-llama-cpp`, file for file as reviewed; the system backends path is empty or absent |
| `local-gguf-models` | every model YAML names a local GGUF file pinned in `Config.ModelFiles` on backend `cpu-llama-cpp`, uses only the reviewed keys (`name`, `backend`, `known_usecases`, `parameters.model`, `context_size`, `mmap`, `stopwords`, `options`, `function.grammar.disable`, `template.use_tokenizer_template`), and lists no option other than `use_jinja` |
| `known-configuration` | every flag and LocalAI variable is one the check evaluates; no `runtime_settings.json` |

Flags and environment variables are both read: LocalAI takes each setting from either, and the check requires every source to be safe. Model YAML is read in a block-mapping subset; a file outside the subset is refused. The listener generation is checked before and after the process is read.

`InspectListener` selects the running listener generation and sends no HTTP request:

```go
selected, err := localai.InspectListener("http://127.0.0.1:18080")
if err != nil { return err }
fmt.Printf("LocalAI PID %d at %s\n", selected.Process.PID, selected.Program)
cfg.ExpectedProcess = selected.Process
```

`ExpectedProcess` carries the exact PID and start time. The OA HTTP guard rechecks that generation before sending request bytes and while reading the response. A LocalAI restart makes the old identity fail; inspect the new listener generation and attach again. Selecting a listener identity does not give OA permission to stop its process.

## Requests

OA sends `Config.APIKey` as a bearer token and forwards only `POST /v1/chat/completions` for allowlisted models. Every other path, including `/models/apply`, `/backends/*` and settings endpoints, is refused before a connection opens. OA performs caller authorization before requests reach LocalAI. The adapter never accepts a caller-supplied HTTP client or router.

## Platforms and limits

Attach works on Linux, where OA and LocalAI share a machine and mount namespace. Windows refuses with `process-observable`; a LocalAI inside WSL reaches Windows through a relay process. macOS refuses at listener inspection, because it cannot bind a loopback connection to its listening process. The check reads the files at attach time: a backend or model file changed afterwards, or a dotenv file removed after LocalAI read it, is not observed. A process that passes can later exec other code; the check holds for the process and files it observed at attach, and LocalAI starts its backend children after that. The attach step hashes the binary, backend and models, which takes a few seconds for a 0.5 GB set.

`TestReviewedPinsAcceptAnInstallation` checks an installation directory against the reviewed pins and the launch recipe when `OA_LOCALAI_INSTALL` names it. It hashes files and starts nothing.

LocalAI owns its backend children, model loading, scheduling and eviction. The attached-server mode has no managed drain guarantee.

Requires Go 1.26 and the OA inference, router, and identity modules.

## Status

The attach conditions follow OA's LocalAI v4.10.0 security review of 2026-10-01.
