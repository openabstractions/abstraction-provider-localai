package localai

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// hp6ModelYAML is the model configuration of the reviewed HP6 run
// (.build/headless-providers/localai/hp6-20260930/qwen3-0.6b-q4_0.yaml).
const hp6ModelYAML = `name: qwen3-0.6b-q4_0
backend: cpu-llama-cpp
known_usecases:
  - chat
parameters:
  model: Qwen3-0.6B-Q4_0.gguf
context_size: 8192
mmap: true
stopwords:
  - <|im_end|>
  - <dummy32000>
  - </s>
  - <|endoftext|>
options:
  - use_jinja:true
function:
  grammar:
    disable: true
template:
  use_tokenizer_template: true
`

const fixtureModelFile = "Qwen3-0.6B-Q4_0.gguf"

// attachFixture is a LocalAI installation laid out under a temporary root
// and a process view that runs LaunchRecipe's command line over it.
type attachFixture struct {
	t    *testing.T
	root string
	view processView
	want attachWant
	pins reviewPins
	key  string
}

func sha(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func newAttachFixture(t *testing.T, port string) *attachFixture {
	t.Helper()
	f := &attachFixture{t: t, root: t.TempDir()}
	exe := []byte("fixture local-ai v4.10.0 image")
	f.write("/task/local-ai", exe)

	f.pins = reviewPins{
		ProgramSHA256: sha(exe),
		BackendName:   "cpu-llama-cpp",
		BackendDigest: reviewedV4100.BackendDigest,
		BackendURI:    reviewedV4100.BackendURI,
		BackendFiles:  map[string]string{"lib": "dir"},
	}
	backendFiles := map[string][]byte{
		"run.sh":                []byte("#!/bin/bash\nexec \"$CURDIR\"/llama-cpp-cpu-all \"$@\"\n"),
		"llama-cpp-cpu-all":     []byte("fixture llama.cpp gRPC server"),
		"lib/libllama.so.0.4.1": []byte("fixture libllama"),
	}
	for rel, data := range backendFiles {
		f.write("/task/backends/cpu-llama-cpp/"+rel, data)
		f.pins.BackendFiles[rel] = sha(data)
	}
	if err := os.Symlink("libllama.so.0.4.1", f.host("/task/backends/cpu-llama-cpp/lib/libllama.so.0")); err == nil {
		f.pins.BackendFiles["lib/libllama.so.0"] = "link:libllama.so.0.4.1"
	}
	f.write("/task/backends/cpu-llama-cpp/metadata.json", []byte(`{
  "name": "cpu-llama-cpp",
  "installed_at": "2026-09-30T10:21:32+03:00",
  "uri": "`+reviewedV4100.BackendURI+`",
  "digest": "`+reviewedV4100.BackendDigest+`"
}`))

	model := append([]byte("GGUF"), []byte("fixture weights")...)
	f.write("/task/models/"+fixtureModelFile, model)
	f.write("/task/models/qwen3-0.6b-q4_0.yaml", []byte(hp6ModelYAML))
	if err := os.MkdirAll(f.host("/task/configuration"), 0o700); err != nil {
		t.Fatal(err)
	}

	key, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	f.key = key
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	launch, err := LaunchRecipe(LaunchPaths{Program: "/task/local-ai", Dir: "/task", Port: portNumber, APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	f.view = processView{Root: f.root, Exe: f.host("/task/local-ai"), Args: launch.Args, Env: launch.Env, Cwd: launch.Dir}
	f.want = attachWant{
		Port:       port,
		APIKey:     key,
		Models:     []string{"qwen3-0.6b-q4_0"},
		ModelFiles: map[string]string{fixtureModelFile: sha(model)},
	}
	return f
}

func (f *attachFixture) host(p string) string { return filepath.Join(f.root, filepath.FromSlash(p)) }

func (f *attachFixture) write(p string, data []byte) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.host(p)), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(f.host(p), data, 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *attachFixture) remove(p string) {
	f.t.Helper()
	if err := os.RemoveAll(f.host(p)); err != nil {
		f.t.Fatal(err)
	}
}

func (f *attachFixture) setArg(prefix, replacement string) {
	f.t.Helper()
	for i, a := range f.view.Args {
		if a == prefix || strings.HasPrefix(a, prefix+"=") {
			if replacement == "" {
				f.view.Args = append(f.view.Args[:i:i], f.view.Args[i+1:]...)
			} else {
				f.view.Args[i] = replacement
			}
			return
		}
	}
	f.t.Fatalf("fixture has no argument %s", prefix)
}

func (f *attachFixture) addArgs(args ...string) { f.view.Args = append(f.view.Args, args...) }

// addModelOption appends an entry to the HP6 model's options list.
func (f *attachFixture) addModelOption(entry string) {
	f.t.Helper()
	yaml := strings.Replace(hp6ModelYAML, "  - use_jinja:true\n", "  - use_jinja:true\n  - "+entry+"\n", 1)
	f.write("/task/models/qwen3-0.6b-q4_0.yaml", []byte(yaml))
}

func (f *attachFixture) setEnv(name, value string) {
	env := f.view.Env[:0:0]
	for _, kv := range f.view.Env {
		if !strings.HasPrefix(kv, name+"=") {
			env = append(env, kv)
		}
	}
	if value != "\x00" {
		env = append(env, name+"="+value)
	}
	f.view.Env = env
}

func (f *attachFixture) unsetEnv(name string) { f.setEnv(name, "\x00") }

func TestAttachAcceptsTheLaunchRecipe(t *testing.T) {
	f := newAttachFixture(t, "18080")
	if all := evaluateAttach(f.view, f.want, f.pins); len(all) != 0 {
		t.Fatalf("recipe fixture refused: %v", all)
	}
}

// TestAttachRefusesEachUnmetCondition checks every refusal twice: the
// mutation is refused with its condition, and with that condition's check
// removed the same process would be accepted. The second half is the proof
// that the test fails without its check.
func TestAttachRefusesEachUnmetCondition(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*attachFixture)
		want   Condition
	}{
		// Process observation and program pin.
		{"executable-unreadable", func(f *attachFixture) { f.view.Exe = f.host("/task/missing") }, ConditionProcessObservable},
		{"executable-not-reviewed", func(f *attachFixture) { f.write("/task/local-ai", []byte("another build")) }, ConditionProgramPin},

		// Loopback listener.
		{"address-default-all-interfaces", func(f *attachFixture) { f.setArg("--address", "") }, ConditionLoopbackListener},
		{"address-wildcard", func(f *attachFixture) { f.setArg("--address", "--address=0.0.0.0:18080") }, ConditionLoopbackListener},
		{"address-lan", func(f *attachFixture) { f.setArg("--address", "--address=192.168.1.5:18080") }, ConditionLoopbackListener},
		{"address-other-port", func(f *attachFixture) { f.setArg("--address", "--address=127.0.0.1:9") }, ConditionLoopbackListener},
		{"address-env-alias", func(f *attachFixture) { f.setEnv("ADDRESS", ":8080") }, ConditionLoopbackListener},
		{"socket-activation", func(f *attachFixture) { f.setEnv("LISTEN_FDS", "1") }, ConditionLoopbackListener},
		{"mitm-listener", func(f *attachFixture) { f.addArgs("--mitm-listen=127.0.0.1:18443") }, ConditionLoopbackListener},
		{"insecure-public-bind", func(f *attachFixture) { f.setEnv("LOCALAI_ALLOW_INSECURE_PUBLIC_BIND", "true") }, ConditionLoopbackListener},

		// API key.
		{"api-key-unset", func(f *attachFixture) { f.unsetEnv("LOCALAI_API_KEY") }, ConditionAPIKey},
		{"api-key-other", func(f *attachFixture) { f.setEnv("LOCALAI_API_KEY", strings.Repeat("a", 64)) }, ConditionAPIKey},
		{"api-key-extra", func(f *attachFixture) { f.setEnv("LOCALAI_API_KEY", f.key+","+strings.Repeat("b", 64)) }, ConditionAPIKey},
		{"api-key-alias-differs", func(f *attachFixture) { f.setEnv("API_KEY", "shared") }, ConditionAPIKey},
		{"api-key-on-command-line", func(f *attachFixture) { f.addArgs("--api-keys=" + f.key) }, ConditionAPIKey},
		{"api-keys-json", func(f *attachFixture) { f.write("/task/configuration/api_keys.json", []byte(`["x"]`)) }, ConditionAPIKey},
		{"get-without-key", func(f *attachFixture) { f.addArgs("--disable-api-key-requirement-for-http-get") }, ConditionAPIKey},

		// Galleries.
		{"galleries-default", func(f *attachFixture) { f.setArg("--galleries", "") }, ConditionGalleriesOff},
		{"galleries-listed", func(f *attachFixture) {
			f.setArg("--galleries", `--galleries=[{"name":"localai","url":"github:mudler/LocalAI/gallery/index.yaml@master"}]`)
		}, ConditionGalleriesOff},
		{"galleries-env-overrides-nothing", func(f *attachFixture) { f.setEnv("LOCALAI_GALLERIES", `[{"url":"x"}]`) }, ConditionGalleriesOff},
		{"backend-galleries-default", func(f *attachFixture) { f.setArg("--backend-galleries", "") }, ConditionGalleriesOff},
		{"autoload-galleries-default", func(f *attachFixture) { f.setArg("--autoload-galleries", "") }, ConditionGalleriesOff},
		{"autoload-backend-galleries-true", func(f *attachFixture) {
			f.setArg("--autoload-backend-galleries", "--autoload-backend-galleries=true")
		}, ConditionGalleriesOff},
		{"autoload-galleries-env-true", func(f *attachFixture) { f.setEnv("AUTOLOAD_GALLERIES", "true") }, ConditionGalleriesOff},
		{"auto-upgrade-backends", func(f *attachFixture) { f.setArg("--auto-upgrade-backends", "--auto-upgrade-backends") }, ConditionGalleriesOff},
		{"preload-models", func(f *attachFixture) { f.addArgs(`--preload-models=[{"url":"github:x"}]`) }, ConditionGalleriesOff},
		{"external-backends", func(f *attachFixture) { f.addArgs("--external-backends=vllm") }, ConditionGalleriesOff},

		// Gallery endpoint.
		{"gallery-endpoint-on", func(f *attachFixture) { f.setArg("--disable-gallery-endpoint", "") }, ConditionGalleryEndpointOff},
		{"gallery-endpoint-env-false", func(f *attachFixture) { f.setEnv("DISABLE_GALLERY_ENDPOINT", "false") }, ConditionGalleryEndpointOff},

		// P2P and federation.
		{"p2p-flag", func(f *attachFixture) { f.addArgs("--p2p") }, ConditionNoPeerNetwork},
		{"p2p-token-env", func(f *attachFixture) { f.setEnv("LOCALAI_P2P_TOKEN", "abc") }, ConditionNoPeerNetwork},
		{"p2p-token-generic-alias", func(f *attachFixture) { f.setEnv("TOKEN", "abc") }, ConditionNoPeerNetwork},
		{"p2p-unknown-variable", func(f *attachFixture) { f.setEnv("LOCALAI_P2P_LOGLEVEL", "debug") }, ConditionNoPeerNetwork},
		{"federated", func(f *attachFixture) { f.setEnv("FEDERATED", "true") }, ConditionNoPeerNetwork},
		{"distributed", func(f *attachFixture) { f.setEnv("LOCALAI_DISTRIBUTED", "true") }, ConditionNoPeerNetwork},

		// Remote backends.
		{"llamacpp-grpc-servers", func(f *attachFixture) { f.setEnv("LLAMACPP_GRPC_SERVERS", "10.0.0.2:50052") }, ConditionNoRemoteBackends},
		{"external-grpc-backend", func(f *attachFixture) { f.addArgs("--external-grpc-backends=llama:10.0.0.2:50051") }, ConditionNoRemoteBackends},
		{"external-backends-json", func(f *attachFixture) {
			f.write("/task/configuration/external_backends.json", []byte(`{"llama":"10.0.0.2:50051"}`))
		}, ConditionNoRemoteBackends},

		// Clean environment.
		{"hf-token", func(f *attachFixture) { f.setEnv("HF_TOKEN", "hf_x") }, ConditionCleanEnvironment},
		{"hf-endpoint", func(f *attachFixture) { f.setEnv("HF_ENDPOINT", "https://mirror.example") }, ConditionCleanEnvironment},
		{"https-proxy", func(f *attachFixture) { f.setEnv("https_proxy", "http://proxy:3128") }, ConditionCleanEnvironment},
		{"ld-preload", func(f *attachFixture) { f.setEnv("LD_PRELOAD", "/tmp/x.so") }, ConditionCleanEnvironment},
		{"bash-env", func(f *attachFixture) { f.setEnv("BASH_ENV", "/tmp/x.sh") }, ConditionCleanEnvironment},
		{"dotenv-in-working-directory", func(f *attachFixture) { f.write("/task/.env", []byte("LOCALAI_P2P=true\n")) }, ConditionCleanEnvironment},
		{"localai-env-in-home", func(f *attachFixture) { f.write("/task/home/.config/localai.env", []byte("GALLERIES=x\n")) }, ConditionCleanEnvironment},
		{"credentials-in-data", func(f *attachFixture) { f.write("/task/data/credentials.yaml", []byte("rules: []\n")) }, ConditionCleanEnvironment},
		{"credentials-flag", func(f *attachFixture) { f.addArgs("--credentials-file=/task/creds.yaml") }, ConditionCleanEnvironment},

		// Backend manifest.
		{"second-backend", func(f *attachFixture) { f.write("/task/backends/vllm/run.sh", []byte("#!/bin/sh\n")) }, ConditionBackendManifest},
		{"backend-file-changed", func(f *attachFixture) {
			f.write("/task/backends/cpu-llama-cpp/run.sh", []byte("#!/bin/bash\nexport LLAMACPP_GRPC_SERVERS=x\n"))
		}, ConditionBackendManifest},
		{"backend-extra-library", func(f *attachFixture) { f.write("/task/backends/cpu-llama-cpp/lib/libevil.so", []byte("x")) }, ConditionBackendManifest},
		{"backend-file-missing", func(f *attachFixture) { f.remove("/task/backends/cpu-llama-cpp/llama-cpp-cpu-all") }, ConditionBackendManifest},
		{"backend-metadata-digest", func(f *attachFixture) {
			f.write("/task/backends/cpu-llama-cpp/metadata.json", []byte(`{"name":"cpu-llama-cpp","digest":"sha256:00","uri":"`+reviewedV4100.BackendURI+`"}`))
		}, ConditionBackendManifest},
		{"backend-metadata-extra-field", func(f *attachFixture) {
			f.write("/task/backends/cpu-llama-cpp/metadata.json", []byte(`{"name":"cpu-llama-cpp","digest":"`+reviewedV4100.BackendDigest+`","uri":"`+reviewedV4100.BackendURI+`","alias":"llama-cpp"}`))
		}, ConditionBackendManifest},
		{"system-backends", func(f *attachFixture) { f.write("/task/system-backends/cuda/run.sh", []byte("x")) }, ConditionBackendManifest},
		{"backends-path-elsewhere", func(f *attachFixture) { f.setArg("--backends-path", "--backends-path=/elsewhere") }, ConditionBackendManifest},

		// Local GGUF models.
		{"model-backend", func(f *attachFixture) {
			f.write("/task/models/qwen3-0.6b-q4_0.yaml", []byte(strings.Replace(hp6ModelYAML, "backend: cpu-llama-cpp", "backend: transformers", 1)))
		}, ConditionLocalModels},
		{"model-url", func(f *attachFixture) {
			f.write("/task/models/qwen3-0.6b-q4_0.yaml", []byte(strings.Replace(hp6ModelYAML, "model: Qwen3-0.6B-Q4_0.gguf", "model: huggingface://Qwen/Qwen3-0.6B-GGUF/Qwen3-0.6B-Q4_0.gguf", 1)))
		}, ConditionLocalModels},
		{"model-trust-remote-code", func(f *attachFixture) {
			f.write("/task/models/qwen3-0.6b-q4_0.yaml", []byte(hp6ModelYAML+"trust_remote_code: true\n"))
		}, ConditionLocalModels},
		{"model-download-files", func(f *attachFixture) {
			f.write("/task/models/qwen3-0.6b-q4_0.yaml", []byte(hp6ModelYAML+"download_files:\n  - x\n"))
		}, ConditionLocalModels},
		{"model-option-rpc-flag", func(f *attachFixture) { f.addModelOption("--rpc:10.0.0.2:50052") }, ConditionNoRemoteBackends},
		{"model-option-grpc-servers", func(f *attachFixture) { f.addModelOption("grpc_servers:10.0.0.2:50052") }, ConditionNoRemoteBackends},
		{"model-option-rpc-servers", func(f *attachFixture) { f.addModelOption("rpc_servers:10.0.0.2:50052") }, ConditionNoRemoteBackends},
		{"model-option-hf-repo-flag", func(f *attachFixture) { f.addModelOption("--hf-repo:Qwen/Qwen3-0.6B-GGUF") }, ConditionLocalModels},
		{"model-option-lora-flag", func(f *attachFixture) { f.addModelOption("--lora:/tmp/adapter.gguf") }, ConditionLocalModels},
		{"model-option-unlisted", func(f *attachFixture) { f.addModelOption("cache_ram:0") }, ConditionLocalModels},
		{"model-env-grpc-servers", func(f *attachFixture) {
			f.write("/task/models/qwen3-0.6b-q4_0.yaml", []byte(hp6ModelYAML+"env:\n  LLAMACPP_GRPC_SERVERS: 10.0.0.2:50052\n"))
		}, ConditionNoRemoteBackends},
		{"model-env-ld-preload", func(f *attachFixture) {
			f.write("/task/models/qwen3-0.6b-q4_0.yaml", []byte(hp6ModelYAML+"env:\n  LD_PRELOAD: /tmp/x.so\n"))
		}, ConditionCleanEnvironment},
		{"model-env-any", func(f *attachFixture) {
			f.write("/task/models/qwen3-0.6b-q4_0.yaml", []byte(hp6ModelYAML+"env:\n  OMP_NUM_THREADS: 4\n"))
		}, ConditionCleanEnvironment},
		{"model-unlisted-key-proxy", func(f *attachFixture) {
			f.write("/task/models/qwen3-0.6b-q4_0.yaml", []byte(hp6ModelYAML+"proxy:\n  mode: passthrough\n"))
		}, ConditionLocalModels},
		{"model-unlisted-key-mcp", func(f *attachFixture) {
			f.write("/task/models/qwen3-0.6b-q4_0.yaml", []byte(hp6ModelYAML+"mcp:\n  stdio: npx\n"))
		}, ConditionLocalModels},
		{"model-unlisted-parameter", func(f *attachFixture) {
			f.write("/task/models/qwen3-0.6b-q4_0.yaml", []byte(strings.Replace(hp6ModelYAML, "  model: Qwen3-0.6B-Q4_0.gguf\n", "  model: Qwen3-0.6B-Q4_0.gguf\n  draft: x.gguf\n", 1)))
		}, ConditionLocalModels},
		{"model-listed-key-bad-value", func(f *attachFixture) {
			f.write("/task/models/qwen3-0.6b-q4_0.yaml", []byte(strings.Replace(hp6ModelYAML, "mmap: true", "mmap: sometimes", 1)))
		}, ConditionLocalModels},
		{"model-flow-yaml", func(f *attachFixture) {
			f.write("/task/models/qwen3-0.6b-q4_0.yaml", []byte(hp6ModelYAML+"extra: {uri: x}\n"))
		}, ConditionLocalModels},
		{"model-second-config-remote", func(f *attachFixture) {
			f.write("/task/models/other.yaml", []byte("name: other\nbackend: cpu-llama-cpp\nparameters:\n  model: https://example.test/x.gguf\n"))
		}, ConditionLocalModels},
		{"model-weights-changed", func(f *attachFixture) { f.write("/task/models/"+fixtureModelFile, []byte("GGUF other weights")) }, ConditionLocalModels},
		{"model-not-gguf", func(f *attachFixture) {
			f.write("/task/models/"+fixtureModelFile, []byte("PK\x03\x04 pickle"))
			f.want.ModelFiles[fixtureModelFile] = sha([]byte("PK\x03\x04 pickle"))
		}, ConditionLocalModels},
		{"model-unpinned", func(f *attachFixture) { delete(f.want.ModelFiles, fixtureModelFile) }, ConditionLocalModels},
		{"model-allowlisted-missing", func(f *attachFixture) { f.want.Models = append(f.want.Models, "absent") }, ConditionLocalModels},
		{"model-stray-checkpoint", func(f *attachFixture) { f.write("/task/models/model.ckpt", []byte("x")) }, ConditionLocalModels},
		{"model-url-positional", func(f *attachFixture) { f.addArgs("huggingface://Qwen/x.gguf") }, ConditionLocalModels},
		{"model-urls-env", func(f *attachFixture) { f.setEnv("LOCALAI_MODELS", "github:x/y.yaml") }, ConditionLocalModels},

		// Configuration OA does not evaluate.
		{"unknown-flag", func(f *attachFixture) { f.addArgs("--agent-hub-url=https://x") }, ConditionKnownConfiguration},
		{"short-flag", func(f *attachFixture) { f.addArgs("-t4") }, ConditionKnownConfiguration},
		{"unknown-localai-variable", func(f *attachFixture) { f.setEnv("LOCALAI_SOMETHING_NEW", "1") }, ConditionKnownConfiguration},
		{"unevaluated-setting", func(f *attachFixture) { f.setEnv("LOCALAI_CORS", "true") }, ConditionKnownConfiguration},
		{"runtime-settings-file", func(f *attachFixture) {
			f.write("/task/configuration/runtime_settings.json", []byte(`{"galleries":[{"url":"x"}]}`))
		}, ConditionKnownConfiguration},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAttachFixture(t, "18080")
			tc.mutate(f)
			err := verifyAttach(f.view, f.want, f.pins)
			var refusal *RefusalError
			if !errors.As(err, &refusal) || !errors.Is(err, ErrRefused) {
				t.Fatalf("verifyAttach = %v, want a refusal", err)
			}
			if refusal.Condition != tc.want {
				t.Fatalf("condition = %s (%s), want %s", refusal.Condition, refusal.Detail, tc.want)
			}
			for _, other := range evaluateAttach(f.view, f.want, f.pins) {
				if other.Condition != tc.want {
					t.Fatalf("without the %s check the process is still refused: %v", tc.want, other)
				}
			}
		})
	}
}

func TestAttachRefusesAChangedBackendLink(t *testing.T) {
	f := newAttachFixture(t, "18080")
	if _, linked := f.pins.BackendFiles["lib/libllama.so.0"]; !linked {
		t.Skip("this account cannot create symlinks")
	}
	f.remove("/task/backends/cpu-llama-cpp/lib/libllama.so.0")
	if err := os.Symlink("/tmp/libllama.so", f.host("/task/backends/cpu-llama-cpp/lib/libllama.so.0")); err != nil {
		t.Fatal(err)
	}
	var refusal *RefusalError
	if err := verifyAttach(f.view, f.want, f.pins); !errors.As(err, &refusal) || refusal.Condition != ConditionBackendManifest {
		t.Fatalf("verifyAttach = %v, want backend-manifest", err)
	}
}

func TestAttachResolvesRelativePathsAgainstTheWorkingDirectory(t *testing.T) {
	f := newAttachFixture(t, "18080")
	f.setArg("--models-path", "--models-path=models")
	f.setArg("--backends-path", "")
	f.setEnv("LOCALAI_BACKENDS_PATH", "./backends")
	if all := evaluateAttach(f.view, f.want, f.pins); len(all) != 0 {
		t.Fatalf("relative paths refused: %v", all)
	}
	f.setEnv("BACKENDS_PATH", "/other")
	var refusal *RefusalError
	if err := verifyAttach(f.view, f.want, f.pins); !errors.As(err, &refusal) || refusal.Condition != ConditionKnownConfiguration {
		t.Fatalf("conflicting backend paths: %v, want known-configuration", err)
	}
}

// TestAttachUsesKongFlagNames checks flags whose kong v1.15.0 name differs
// from the CLI reference document or from a guessed spelling.
func TestAttachUsesKongFlagNames(t *testing.T) {
	f := newAttachFixture(t, "18080")
	f.addArgs("--disable-web-ui")
	if all := evaluateAttach(f.view, f.want, f.pins); len(all) != 0 {
		t.Fatalf("--disable-web-ui refused: %v", all)
	}
	for flag, want := range map[string]Condition{
		"--peer-2-peer-network-id=lan": ConditionNoPeerNetwork,
		"--mitmca-dir=/task/ca":        ConditionLoopbackListener,
		"--disable-webui":              ConditionKnownConfiguration,
	} {
		f := newAttachFixture(t, "18080")
		f.addArgs(flag)
		var refusal *RefusalError
		if err := verifyAttach(f.view, f.want, f.pins); !errors.As(err, &refusal) || refusal.Condition != want {
			t.Fatalf("%s: %v, want %s", flag, err, want)
		}
	}
}

func TestPositionalRefusalDoesNotEchoTheArgument(t *testing.T) {
	f := newAttachFixture(t, "18080")
	f.addArgs("https://user:hunter2@models.example/x.yaml")
	err := verifyAttach(f.view, f.want, f.pins)
	var refusal *RefusalError
	if !errors.As(err, &refusal) || refusal.Condition != ConditionLocalModels {
		t.Fatalf("verifyAttach = %v, want local-gguf-models", err)
	}
	if strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), "models.example") {
		t.Fatalf("refusal echoes the argument: %v", err)
	}
}

// TestCwdInRoot covers a chrooted LocalAI: /proc/<pid>/cwd names the host
// path, and the check needs it inside the process's root.
func TestCwdInRoot(t *testing.T) {
	for _, tc := range []struct {
		root, cwd, want string
		ok              bool
	}{
		{"/", "/srv/localai", "/srv/localai", true},
		{"/jail", "/jail/srv/localai", "/srv/localai", true},
		{"/jail", "/jail", "/", true},
		{"/jail", "/jailbreak/x", "", false},
		{"/jail", "/srv/localai", "", false},
	} {
		got, ok := cwdInRoot(tc.root, tc.cwd)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("cwdInRoot(%q, %q) = %q, %t; want %q, %t", tc.root, tc.cwd, got, ok, tc.want, tc.ok)
		}
	}
}

func TestReviewedPinsMatchTheReview(t *testing.T) {
	if reviewedV4100.ProgramSHA256 != "412cde3dc89b0bf3c643e9c07c40a32cad0d17a1008f818cd4243bd03be8c006" {
		t.Fatal("program pin differs from the review")
	}
	if got := reviewedV4100.BackendFiles["run.sh"]; got != "d74a3bb9b53bd833dc11ea4f820d324977d24d51135c43a34fc67ce549dad568" {
		t.Fatalf("run.sh pin %s differs from the review", got)
	}
	if !strings.HasSuffix(reviewedV4100.BackendURI, "@"+reviewedV4100.BackendDigest) {
		t.Fatal("backend URI does not name the reviewed digest")
	}
	files := 0
	for _, v := range reviewedV4100.BackendFiles {
		if v != "dir" && !strings.HasPrefix(v, "link:") {
			files++
		}
	}
	// run.sh, three binaries and 28 library files; with 10 symlinks lib/
	// holds the review's 38 entries. metadata.json is compared by field.
	if files != 32 {
		t.Fatalf("reviewed backend has %d regular files, want 32", files)
	}
}
