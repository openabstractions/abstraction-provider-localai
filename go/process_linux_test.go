//go:build linux

package localai

import (
	"os"
	"strconv"
	"testing"
)

// TestProcObserveResolvesInTheProcessRoot observes the test process: paths
// resolve under /proc/<pid>/root, and the working directory is expressed in
// that root.
func TestProcObserveResolvesInTheProcessRoot(t *testing.T) {
	view, err := procObserve(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if want := "/proc/" + strconv.Itoa(os.Getpid()) + "/root"; view.Root != want {
		t.Fatalf("Root = %q, want %q", view.Root, want)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if view.Cwd != wd {
		t.Fatalf("Cwd = %q, want %q", view.Cwd, wd)
	}
	viaRoot, err := os.Stat(view.host(view.Cwd))
	if err != nil {
		t.Fatal(err)
	}
	direct, err := os.Stat(wd)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(viaRoot, direct) {
		t.Fatal("working directory resolved through Root is a different directory")
	}
}
