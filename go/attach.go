package localai

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	contentdigest "github.com/openabstractions/abstraction-storage/go/digest"
)

// Condition names one attach condition of the LocalAI v4.10.0 security review
// (research/security-reviews/localai-4.10.0.md). A refusal carries the
// condition the observed process does not meet.
type Condition string

const (
	// ConditionProcessObservable: OA reads the listener's executable,
	// command line, environment and working directory. Only Linux exposes
	// them; a process owned by another user is unreadable.
	ConditionProcessObservable Condition = "process-observable"
	// ConditionProgramPin: the running executable's SHA-256 equals the
	// reviewed release binary.
	ConditionProgramPin Condition = "program-pin"
	// ConditionLoopbackListener: the API binds 127.0.0.1 on the configured
	// port only, with no socket activation, MITM listener or public-bind
	// override.
	ConditionLoopbackListener Condition = "loopback-listener"
	// ConditionAPIKey: exactly OA's per-launch key, from the environment.
	ConditionAPIKey Condition = "api-key"
	// ConditionGalleriesOff: empty model and backend galleries, no gallery
	// autoload or backend auto-upgrade, and nothing installed at start.
	ConditionGalleriesOff Condition = "galleries-off"
	// ConditionGalleryEndpointOff: the gallery HTTP endpoints are disabled.
	ConditionGalleryEndpointOff Condition = "gallery-endpoint-off"
	// ConditionNoPeerNetwork: no P2P, federation or distributed mode.
	ConditionNoPeerNetwork Condition = "no-p2p-federation"
	// ConditionNoRemoteBackends: no LLAMACPP_GRPC_SERVERS and no external
	// gRPC backends.
	ConditionNoRemoteBackends Condition = "no-remote-backends"
	// ConditionCleanEnvironment: no download tokens, proxies, loader or
	// shell injection variables, env files or credential files.
	ConditionCleanEnvironment Condition = "clean-environment"
	// ConditionBackendManifest: the backend directories hold only the
	// reviewed cpu-llama-cpp backend, file for file.
	ConditionBackendManifest Condition = "backend-manifest"
	// ConditionLocalModels: every model configuration names a pinned local
	// GGUF file on the cpu-llama-cpp backend.
	ConditionLocalModels Condition = "local-gguf-models"
	// ConditionKnownConfiguration: every flag and LocalAI setting present is
	// one this check evaluates.
	ConditionKnownConfiguration Condition = "known-configuration"
)

// ErrRefused matches every attach refusal with errors.Is.
var ErrRefused = errors.New("localai: attach refused")

// RefusalError reports the first unmet attach condition.
type RefusalError struct {
	Condition Condition
	Detail    string
}

func (e *RefusalError) Error() string {
	return fmt.Sprintf("localai: attach refused: %s: %s", e.Condition, e.Detail)
}

func (e *RefusalError) Is(target error) bool { return target == ErrRefused }

func refuse(c Condition, format string, args ...any) *RefusalError {
	return &RefusalError{Condition: c, Detail: fmt.Sprintf(format, args...)}
}

// processView is what OA observed of the listener process. Paths in Args,
// Env and Cwd are in the process's own filesystem view; Root is the host
// path under which that view resolves (/proc/<pid>/root on Linux, which is
// the process's chroot when it has one; a fixture directory in tests).
type processView struct {
	Root string
	Exe  string // host path of the running executable image
	Args []string
	Env  []string
	Cwd  string
}

// cwdInRoot expresses a working directory read from /proc/<pid>/cwd, which
// names the host path, inside the process root read from /proc/<pid>/root.
func cwdInRoot(root, cwd string) (string, bool) {
	root, cwd = path.Clean(root), path.Clean(cwd)
	if root == "/" {
		return cwd, true
	}
	if cwd == root {
		return "/", true
	}
	rel, ok := strings.CutPrefix(cwd, root+"/")
	if !ok {
		return "", false
	}
	return "/" + rel, true
}

func (v processView) host(p string) string {
	return filepath.Join(v.Root, filepath.FromSlash(p))
}

// attachWant is what OA itself configured for this attachment.
type attachWant struct {
	Port       string
	APIKey     string
	Models     []string
	ModelFiles map[string]string
}

// verifyAttach returns the first refusal, or nil when the process meets
// every condition.
func verifyAttach(v processView, want attachWant, pins reviewPins) error {
	if all := evaluateAttach(v, want, pins); len(all) > 0 {
		return all[0]
	}
	return nil
}

// evaluateAttach runs every check and returns all refusals in check order.
// Checks that need an effective configuration stop after a configuration
// refusal that leaves their inputs undefined.
func evaluateAttach(v processView, want attachWant, pins reviewPins) []*RefusalError {
	var out []*RefusalError
	add := func(r ...*RefusalError) { out = append(out, r...) }

	add(checkProgram(v, pins)...)
	cfg, refusals := evaluateConfiguration(v, want)
	add(refusals...)
	add(checkEnvironment(v)...)
	if cfg == nil {
		return out
	}
	add(checkEnvFiles(v, cfg)...)
	add(checkConfigDir(v, cfg)...)
	add(checkBackends(v, cfg, pins)...)
	add(checkModels(v, cfg, want)...)
	return out
}

func checkProgram(v processView, pins reviewPins) []*RefusalError {
	sum, err := fileHex(v.Exe)
	if err != nil {
		return []*RefusalError{refuse(ConditionProcessObservable, "read executable: %v", err)}
	}
	if sum != pins.ProgramSHA256 {
		return []*RefusalError{refuse(ConditionProgramPin, "executable SHA-256 %s is not the reviewed LocalAI v4.10.0 binary", sum)}
	}
	return nil
}

// forbiddenEnv lists variables outside LocalAI's own settings that change
// where LocalAI or its backend sends work, what it loads, or who it trusts.
func forbiddenEnv(name string) (Condition, bool) {
	upper := strings.ToUpper(name)
	switch upper {
	case "LLAMACPP_GRPC_SERVERS", "MLX_DISTRIBUTED_HOSTFILE":
		return ConditionNoRemoteBackends, true
	case "LISTEN_FDS", "LISTEN_PID", "LISTEN_FDNAMES":
		return ConditionLoopbackListener, true
	case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "FTP_PROXY", "GRPC_PROXY",
		"BASH_ENV", "ENV", "HUGGING_FACE_HUB_TOKEN", "HUGGINGFACE_HUB_TOKEN", "DOCKER_CONFIG":
		return ConditionCleanEnvironment, true
	}
	switch {
	case strings.HasPrefix(upper, "LOCALAI_P2P"), strings.HasPrefix(upper, "P2P_"):
		return ConditionNoPeerNetwork, true
	case strings.HasPrefix(upper, "HF_"), strings.HasPrefix(upper, "LD_"), strings.HasPrefix(upper, "BASH_FUNC_"):
		return ConditionCleanEnvironment, true
	}
	return "", false
}

func checkEnvironment(v processView) []*RefusalError {
	var out []*RefusalError
	for _, kv := range v.Env {
		name, _, _ := strings.Cut(kv, "=")
		if c, bad := forbiddenEnv(name); bad {
			out = append(out, refuse(c, "environment sets %s", name))
			continue
		}
		if strings.HasPrefix(name, "LOCALAI_") && settingForEnv(name) == nil {
			out = append(out, refuse(ConditionKnownConfiguration, "environment sets %s, which LocalAI v4.10.0 does not define as a setting", name))
		}
	}
	return out
}

// checkEnvFiles refuses the dotenv files cmd/local-ai/main.go loads before
// parsing flags; their variables never appear in the process environment
// OA reads.
func checkEnvFiles(v processView, cfg *effectiveConfig) []*RefusalError {
	candidates := []string{path.Join(v.Cwd, ".env"), path.Join(v.Cwd, "localai.env"), "/etc/localai.env"}
	if home := envValue(v.Env, "HOME"); home != "" {
		candidates = append(candidates, path.Join(home, "localai.env"), path.Join(home, ".config", "localai.env"))
	}
	candidates = append(candidates, path.Join(cfg.DataPath, "credentials.yaml"))
	var out []*RefusalError
	for _, p := range candidates {
		if _, err := os.Lstat(v.host(p)); err == nil {
			out = append(out, refuse(ConditionCleanEnvironment, "%s exists; LocalAI loads it at start", p))
		} else if !errors.Is(err, os.ErrNotExist) {
			out = append(out, refuse(ConditionProcessObservable, "inspect %s: %v", p, err))
		}
	}
	return out
}

// checkConfigDir refuses the dynamic files core/application/
// config_file_watcher.go reads and watches in --localai-config-dir.
func checkConfigDir(v processView, cfg *effectiveConfig) []*RefusalError {
	files := []struct {
		name string
		c    Condition
		why  string
	}{
		{"runtime_settings.json", ConditionKnownConfiguration, "overrides galleries, P2P and other settings at start"},
		{"external_backends.json", ConditionNoRemoteBackends, "adds external gRPC backends"},
		{"api_keys.json", ConditionAPIKey, "adds API keys beyond OA's"},
	}
	var out []*RefusalError
	for _, f := range files {
		p := path.Join(cfg.ConfigDir, f.name)
		if _, err := os.Lstat(v.host(p)); err == nil {
			out = append(out, refuse(f.c, "%s exists; LocalAI %s", p, f.why))
		} else if !errors.Is(err, os.ErrNotExist) {
			out = append(out, refuse(ConditionProcessObservable, "inspect %s: %v", p, err))
		}
	}
	return out
}

func checkBackends(v processView, cfg *effectiveConfig, pins reviewPins) []*RefusalError {
	var out []*RefusalError
	if r := checkEmptyOrAbsent(v, cfg.BackendsSystemPath); r != nil {
		out = append(out, r)
	}
	root := cfg.BackendsPath
	info, err := os.Lstat(v.host(root))
	if err != nil || !info.IsDir() {
		return append(out, refuse(ConditionBackendManifest, "backends path %s is not a directory", root))
	}
	entries, err := os.ReadDir(v.host(root))
	if err != nil {
		return append(out, refuse(ConditionProcessObservable, "read %s: %v", root, err))
	}
	if len(entries) != 1 || entries[0].Name() != pins.BackendName || !entries[0].IsDir() || entries[0].Type()&os.ModeSymlink != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return append(out, refuse(ConditionBackendManifest, "backends path %s holds %v; want only %s", root, names, pins.BackendName))
	}
	dir := path.Join(root, pins.BackendName)
	seen := map[string]bool{}
	walkErr := filepath.WalkDir(v.host(dir), func(hostPath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(v.host(dir), hostPath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		seen[rel] = true
		want, pinned := pins.BackendFiles[rel]
		if rel == "metadata.json" {
			if r := checkBackendMetadata(hostPath, pins); r != nil {
				out = append(out, r)
			}
			return nil
		}
		if !pinned {
			out = append(out, refuse(ConditionBackendManifest, "%s/%s is not in the reviewed backend manifest", dir, rel))
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case d.Type()&os.ModeSymlink != 0:
			target, err := os.Readlink(hostPath)
			if err != nil || "link:"+target != want {
				out = append(out, refuse(ConditionBackendManifest, "%s/%s links to %q; reviewed %q", dir, rel, target, strings.TrimPrefix(want, "link:")))
			}
		case d.IsDir():
			if want != "dir" {
				out = append(out, refuse(ConditionBackendManifest, "%s/%s is a directory", dir, rel))
			}
		case d.Type().IsRegular():
			sum, err := fileHex(hostPath)
			if err != nil {
				out = append(out, refuse(ConditionProcessObservable, "read %s/%s: %v", dir, rel, err))
			} else if sum != want {
				out = append(out, refuse(ConditionBackendManifest, "%s/%s SHA-256 %s differs from the reviewed backend", dir, rel, sum))
			}
		default:
			out = append(out, refuse(ConditionBackendManifest, "%s/%s is not a regular file", dir, rel))
		}
		return nil
	})
	if walkErr != nil {
		out = append(out, refuse(ConditionProcessObservable, "walk %s: %v", dir, walkErr))
	}
	if !seen["metadata.json"] {
		out = append(out, refuse(ConditionBackendManifest, "%s/metadata.json is missing", dir))
	}
	for rel := range pins.BackendFiles {
		if !seen[rel] {
			out = append(out, refuse(ConditionBackendManifest, "%s/%s is missing", dir, rel))
		}
	}
	return out
}

// checkBackendMetadata compares the install record LocalAI wrote. Its
// installed_at differs per install, so the file is compared by field.
func checkBackendMetadata(hostPath string, pins reviewPins) *RefusalError {
	data, err := os.ReadFile(hostPath)
	if err != nil {
		return refuse(ConditionProcessObservable, "read backend metadata: %v", err)
	}
	var meta map[string]any
	if err := json.Unmarshal(data, &meta); err != nil {
		return refuse(ConditionBackendManifest, "backend metadata.json is not a JSON object: %v", err)
	}
	for key, value := range meta {
		s, _ := value.(string)
		switch key {
		case "name":
			if s != pins.BackendName {
				return refuse(ConditionBackendManifest, "backend metadata name %q", s)
			}
		case "digest":
			if s != pins.BackendDigest {
				return refuse(ConditionBackendManifest, "backend metadata digest %q is not the reviewed %s", s, pins.BackendDigest)
			}
		case "uri":
			if s != pins.BackendURI {
				return refuse(ConditionBackendManifest, "backend metadata uri %q is not the reviewed OCI digest", s)
			}
		case "installed_at":
		default:
			return refuse(ConditionBackendManifest, "backend metadata has unreviewed field %q", key)
		}
	}
	if meta["digest"] == nil || meta["name"] == nil {
		return refuse(ConditionBackendManifest, "backend metadata lacks name or digest")
	}
	return nil
}

func checkEmptyOrAbsent(v processView, p string) *RefusalError {
	entries, err := os.ReadDir(v.host(p))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return refuse(ConditionProcessObservable, "read %s: %v", p, err)
	}
	if len(entries) > 0 {
		return refuse(ConditionBackendManifest, "system backends path %s is not empty", p)
	}
	return nil
}

func checkModels(v processView, cfg *effectiveConfig, want attachWant) []*RefusalError {
	var out []*RefusalError
	dir := cfg.ModelsPath
	entries, err := os.ReadDir(v.host(dir))
	if err != nil {
		return []*RefusalError{refuse(ConditionLocalModels, "read models path %s: %v", dir, err)}
	}
	configured := map[string]bool{}
	referenced := map[string]bool{}
	unreadable := false
	for _, e := range entries {
		name := e.Name()
		p := path.Join(dir, name)
		if !e.Type().IsRegular() {
			out = append(out, refuse(ConditionLocalModels, "%s is not a regular file", p))
			continue
		}
		ext := strings.ToLower(path.Ext(name))
		switch {
		case strings.HasPrefix(name, "."):
			out = append(out, refuse(ConditionLocalModels, "%s is an unreviewed hidden file", p))
		case ext == ".yaml" || ext == ".yml":
			data, err := os.ReadFile(v.host(p))
			if err != nil {
				out = append(out, refuse(ConditionProcessObservable, "read %s: %v", p, err))
				continue
			}
			model, r := checkModelConfig(p, data)
			if r != nil {
				out = append(out, r)
				unreadable = true
				continue
			}
			if configured[model.Name] {
				out = append(out, refuse(ConditionLocalModels, "model %q is configured twice", model.Name))
			}
			configured[model.Name] = true
			referenced[model.File] = true
		case ext == ".gguf":
		default:
			out = append(out, refuse(ConditionLocalModels, "%s is neither a model configuration nor a GGUF file", p))
		}
	}
	for file := range referenced {
		wantSum, pinned := want.ModelFiles[file]
		p := path.Join(dir, file)
		if !pinned {
			out = append(out, refuse(ConditionLocalModels, "%s has no pinned SHA-256 in OA's configuration", p))
			continue
		}
		if r := checkGGUF(v.host(p), p, wantSum); r != nil {
			out = append(out, r)
		}
	}
	if unreadable {
		// A refused configuration names no files and no model; the cross
		// references below would repeat that refusal.
		return out
	}
	for _, e := range entries {
		if strings.EqualFold(path.Ext(e.Name()), ".gguf") && !referenced[e.Name()] {
			out = append(out, refuse(ConditionLocalModels, "%s is not named by any model configuration", path.Join(dir, e.Name())))
		}
	}
	for _, m := range want.Models {
		if !configured[m] {
			out = append(out, refuse(ConditionLocalModels, "allowlisted model %q has no configuration in %s", m, dir))
		}
	}
	return out
}

func checkGGUF(hostPath, shown, wantSum string) *RefusalError {
	f, err := os.Open(hostPath)
	if err != nil {
		return refuse(ConditionLocalModels, "open %s: %v", shown, err)
	}
	defer f.Close()
	h := sha256.New()
	magic := make([]byte, 4)
	if _, err := io.ReadFull(f, magic); err != nil || string(magic) != "GGUF" {
		return refuse(ConditionLocalModels, "%s is not a GGUF file", shown)
	}
	h.Write(magic)
	if _, err := io.Copy(h, f); err != nil {
		return refuse(ConditionProcessObservable, "read %s: %v", shown, err)
	}
	if sum := hex.EncodeToString(h.Sum(nil)); sum != strings.ToLower(wantSum) {
		return refuse(ConditionLocalModels, "%s SHA-256 %s differs from the pinned %s", shown, sum, wantSum)
	}
	return nil
}

// fileHex returns the file's SHA-256 as the bare hex the review pins hold.
func fileHex(p string) (string, error) {
	_, d, err := contentdigest.File(context.Background(), p, nil)
	return strings.TrimPrefix(d, contentdigest.Prefix), err
}

func envValue(env []string, name string) string {
	value := ""
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			value = v
		}
	}
	return value
}

// loopbackAddress accepts exactly 127.0.0.1:<port>.
func loopbackAddress(value, port string) bool {
	host, p, err := net.SplitHostPort(value)
	if err != nil || host != "127.0.0.1" {
		return false
	}
	n, err := strconv.Atoi(p)
	return err == nil && n > 0 && n < 65536 && p == port
}

func equalSecret(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
