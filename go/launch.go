package localai

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path"
	"regexp"
	"strconv"
)

// LaunchPaths names what the launch recipe needs. Paths are absolute Linux
// paths: the reviewed LocalAI is local-ai-v4.10.0-linux-amd64.
type LaunchPaths struct {
	// Program is the reviewed local-ai binary.
	Program string
	// Dir holds every LocalAI path: models/, backends/, data/,
	// configuration/, generated/, upload/, home/ and tmp/.
	Dir string
	// Port is the loopback port OA attaches to.
	Port int
	// APIKey is the per-launch key from NewAPIKey.
	APIKey string
}

// Launch is a LocalAI command line and complete environment that meet every
// attach condition. Start the program with exactly Env, not an extension of
// the caller's environment, in working directory Dir.
type Launch struct {
	Args []string
	Env  []string
	Dir  string
}

var apiKeyShape = regexp.MustCompile(`^[A-Za-z0-9_-]{32,256}$`)

// NewAPIKey returns a random per-launch API key.
func NewAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// LaunchRecipe returns the LocalAI v4.10.0 launch that the attach check
// accepts. Flag and variable names are from core/cli/run.go and
// LocalAI/docs/content/reference/cli-reference.md at tag commit
// 7ad0cbf259f0c7bf9920fe2438fc3630ecd6c672:
//
//   - --address=127.0.0.1:<port> replaces the default :8080 on every interface.
//   - LOCALAI_API_KEY carries the key in the environment, which only the
//     process owner can read; a --api-keys value is visible to every user.
//   - --galleries=[] and --backend-galleries=[] replace the defaults, which
//     point at GitHub master; --autoload-galleries=false and
//     --autoload-backend-galleries=false stop the start-time index fetch and
//     remote VRAM probes (core/application/startup.go).
//   - --disable-gallery-endpoint removes the gallery HTTP endpoints, and
//     --auto-upgrade-backends=false keeps the six-hourly upgrade off.
//   - Every path flag points inside Dir, and --backends-system-path replaces
//     /var/lib/local-ai/backends.
//   - HOME and the working directory are inside Dir, so the dotenv files
//     cmd/local-ai/main.go loads are under OA's control; PATH lists system
//     directories only, for the backend's run.sh.
//   - LOCALAI_DISABLE_AGENTS, LOCALAI_DISABLE_MCP, LOCALAI_DISABLE_ASSISTANT,
//     LOCALAI_DISABLE_WEBUI and LOCALAI_DISABLE_RUNTIME_SETTINGS turn off
//     surfaces OA does not use.
//
// P2P, federation, MITM, external backends, preloads, model URLs, HF_TOKEN,
// proxies and LLAMACPP_GRPC_SERVERS are absent.
func LaunchRecipe(p LaunchPaths) (Launch, error) {
	if !path.IsAbs(p.Program) || !path.IsAbs(p.Dir) || path.Clean(p.Dir) != p.Dir || p.Dir == "/" {
		return Launch{}, errors.New("localai: launch recipe needs an absolute program and a clean absolute directory")
	}
	if p.Port <= 0 || p.Port > 65535 {
		return Launch{}, errors.New("localai: launch recipe needs a TCP port")
	}
	if !apiKeyShape.MatchString(p.APIKey) {
		return Launch{}, errors.New("localai: launch recipe needs a per-launch API key from NewAPIKey")
	}
	d := func(name string) string { return path.Join(p.Dir, name) }
	return Launch{
		Args: []string{
			p.Program, "run",
			"--address=127.0.0.1:" + strconv.Itoa(p.Port),
			"--models-path=" + d("models"),
			"--backends-path=" + d("backends"),
			"--backends-system-path=" + d("system-backends"),
			"--data-path=" + d("data"),
			"--localai-config-dir=" + d("configuration"),
			"--generated-content-path=" + d("generated"),
			"--upload-path=" + d("upload"),
			"--galleries=[]",
			"--backend-galleries=[]",
			"--autoload-galleries=false",
			"--autoload-backend-galleries=false",
			"--disable-gallery-endpoint",
			"--auto-upgrade-backends=false",
		},
		Env: []string{
			"PATH=/usr/local/bin:/usr/bin:/bin",
			"HOME=" + d("home"),
			"TMPDIR=" + d("tmp"),
			"LOCALAI_API_KEY=" + p.APIKey,
			"LOCALAI_DISABLE_AGENTS=true",
			"LOCALAI_DISABLE_MCP=true",
			"LOCALAI_DISABLE_ASSISTANT=true",
			"LOCALAI_DISABLE_WEBUI=true",
			"LOCALAI_DISABLE_RUNTIME_SETTINGS=true",
		},
		Dir: p.Dir,
	}, nil
}
