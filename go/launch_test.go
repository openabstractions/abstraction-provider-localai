package localai

import (
	"strings"
	"testing"
)

func TestLaunchRecipeKeepsTheKeyOffTheCommandLine(t *testing.T) {
	key, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	launch, err := LaunchRecipe(LaunchPaths{Program: "/opt/localai/local-ai", Dir: "/srv/localai", Port: 18080, APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range launch.Args {
		if strings.Contains(a, key) {
			t.Fatalf("argument %q carries the API key", a)
		}
	}
	if launch.Args[0] != "/opt/localai/local-ai" || launch.Args[1] != "run" || launch.Dir != "/srv/localai" {
		t.Fatalf("unexpected launch %+v", launch)
	}
	if envValue(launch.Env, "LOCALAI_API_KEY") != key {
		t.Fatal("recipe environment lacks the key")
	}
}

func TestLaunchRecipeRefusesUnsafeInputs(t *testing.T) {
	key := strings.Repeat("a", 64)
	for _, p := range []LaunchPaths{
		{Program: "local-ai", Dir: "/srv/localai", Port: 18080, APIKey: key},
		{Program: "/opt/local-ai", Dir: "srv", Port: 18080, APIKey: key},
		{Program: "/opt/local-ai", Dir: "/", Port: 18080, APIKey: key},
		{Program: "/opt/local-ai", Dir: "/srv/../etc", Port: 18080, APIKey: key},
		{Program: "/opt/local-ai", Dir: "/srv/localai", Port: 0, APIKey: key},
		{Program: "/opt/local-ai", Dir: "/srv/localai", Port: 18080, APIKey: "short"},
		{Program: "/opt/local-ai", Dir: "/srv/localai", Port: 18080, APIKey: key + ",other-key-for-someone-else-000000"},
	} {
		if _, err := LaunchRecipe(p); err == nil {
			t.Fatalf("LaunchRecipe(%+v) accepted", p)
		}
	}
}

func TestModelYAMLSubset(t *testing.T) {
	m, r := checkModelConfig("hp6.yaml", []byte(hp6ModelYAML))
	if r != nil {
		t.Fatal(r)
	}
	if m.Name != "qwen3-0.6b-q4_0" || m.File != "Qwen3-0.6B-Q4_0.gguf" {
		t.Fatalf("model = %+v", m)
	}
	for name, yaml := range map[string]string{
		"anchor":         "name: a\nbackend: &b cpu-llama-cpp\n",
		"block-scalar":   "name: a\nbackend: cpu-llama-cpp\ndescription: |\n  x\n",
		"document":       "---\nname: a\n",
		"duplicate":      "name: a\nname: b\n",
		"list-mapping":   "name: a\nbackend: cpu-llama-cpp\nparameters:\n  model: x.gguf\nfiles:\n  - filename: x\n",
		"tab":            "name: a\n\tbackend: x\n",
		"array-document": "- name: a\n",
		"nested-uri":     "name: a\nbackend: cpu-llama-cpp\nparameters:\n  model: x.gguf\nmmproj: y.gguf\n",
		"path-traversal": "name: a\nbackend: cpu-llama-cpp\nparameters:\n  model: ../x.gguf\n",
		"subdirectory":   "name: a\nbackend: cpu-llama-cpp\nparameters:\n  model: sub/x.gguf\n",
		"safetensors":    "name: a\nbackend: cpu-llama-cpp\nparameters:\n  model: x.safetensors\n",
		"github-uri":     "name: a\nbackend: cpu-llama-cpp\nparameters:\n  model: x.gguf\ndescription: github:mudler/x\n",
	} {
		if _, r := checkModelConfig(name, []byte(yaml)); r == nil || r.Condition != ConditionLocalModels {
			t.Fatalf("%s accepted or misclassified: %v", name, r)
		}
	}
}
