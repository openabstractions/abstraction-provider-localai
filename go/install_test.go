package localai

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// hp6ModelPins are the model files of the reviewed HP6 installation.
var hp6ModelPins = map[string]string{
	"Qwen3-0.6B-Q4_0.gguf": "33bcc57074ec7b6eada5a90651ee546ec0c2b271002c22baf9f1b2dd1e8f75cb",
}

// TestReviewedPinsAcceptAnInstallation evaluates a real LocalAI installation
// directory (the local-ai binary, backends/ and models/ as LaunchRecipe lays
// them out) against the reviewed pins and LaunchRecipe's command line. It
// hashes files only; nothing is started. Set OA_LOCALAI_INSTALL to the
// directory, for example the HP6 run through
// \\wsl.localhost\Ubuntu\root\.cache\openabstractions\hp6-localai-20260930.
func TestReviewedPinsAcceptAnInstallation(t *testing.T) {
	dir := os.Getenv("OA_LOCALAI_INSTALL")
	if dir == "" {
		t.Skip("OA_LOCALAI_INSTALL is not set")
	}
	dir = filepath.Clean(dir)
	processDir := "/" + filepath.Base(dir)
	key, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	launch, err := LaunchRecipe(LaunchPaths{Program: processDir + "/local-ai", Dir: processDir, Port: 18080, APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	view := processView{
		Root: filepath.Dir(dir),
		Exe:  filepath.Join(dir, "local-ai"),
		Args: launch.Args,
		Env:  launch.Env,
		Cwd:  launch.Dir,
	}
	want := attachWant{Port: "18080", APIKey: key, Models: []string{"qwen3-0.6b-q4_0"}, ModelFiles: hp6ModelPins}
	unverified := 0
	for _, r := range evaluateAttach(view, want, reviewedV4100) {
		if windowsLXSymlink(r, processDir) {
			// Windows reads a WSL symlink over \\wsl.localhost as an
			// irregular file and cannot read its target.
			unverified++
			continue
		}
		t.Error(r)
	}
	if unverified > 0 {
		t.Logf("%d pinned symlinks were not verifiable from Windows", unverified)
	}
}

func windowsLXSymlink(r *RefusalError, processDir string) bool {
	if runtime.GOOS != "windows" || r.Condition != ConditionBackendManifest {
		return false
	}
	prefix := processDir + "/backends/" + reviewedV4100.BackendName + "/"
	rel, ok := strings.CutSuffix(strings.TrimPrefix(r.Detail, prefix), " is not a regular file")
	return ok && strings.HasPrefix(reviewedV4100.BackendFiles[rel], "link:")
}
