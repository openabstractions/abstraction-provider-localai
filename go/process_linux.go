//go:build linux

package localai

import (
	"bytes"
	"os"
	"strconv"
)

// procObserve reads the listener's executable image, command line,
// environment and working directory from procfs. environ, cwd, exe and root
// need the process's owner or CAP_SYS_PTRACE; an unreadable process refuses.
// The executable is hashed through /proc/<pid>/exe, the image the process
// runs even after its path is replaced.
func procObserve(pid int) (processView, error) {
	base := "/proc/" + strconv.Itoa(pid)
	cmdline, err := os.ReadFile(base + "/cmdline")
	if err != nil {
		return processView{}, refuse(ConditionProcessObservable, "read command line: %v", err)
	}
	environ, err := os.ReadFile(base + "/environ")
	if err != nil {
		return processView{}, refuse(ConditionProcessObservable, "read environment: %v", err)
	}
	cwd, err := os.Readlink(base + "/cwd")
	if err != nil {
		return processView{}, refuse(ConditionProcessObservable, "read working directory: %v", err)
	}
	if _, err := os.Stat(base + "/exe"); err != nil {
		return processView{}, refuse(ConditionProcessObservable, "open executable: %v", err)
	}
	// Paths in the command line resolve in the process's mount namespace.
	// OA inspects them in its own, so the two must be the same.
	theirs, err := os.Readlink(base + "/ns/mnt")
	if err != nil {
		return processView{}, refuse(ConditionProcessObservable, "read mount namespace: %v", err)
	}
	ours, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil || ours != theirs {
		return processView{}, refuse(ConditionProcessObservable, "LocalAI runs in mount namespace %s; OA in %s", theirs, ours)
	}
	// A chrooted LocalAI reads its paths inside its root. /proc/<pid>/root
	// resolves there; /proc/<pid>/cwd names the host path, which is
	// re-expressed inside that root.
	root, err := os.Readlink(base + "/root")
	if err != nil {
		return processView{}, refuse(ConditionProcessObservable, "read root directory: %v", err)
	}
	inRoot, ok := cwdInRoot(root, cwd)
	if !ok {
		return processView{}, refuse(ConditionProcessObservable, "working directory %s is outside the process root %s", cwd, root)
	}
	return processView{
		Root: base + "/root",
		Exe:  base + "/exe",
		Args: splitNUL(cmdline),
		Env:  splitNUL(environ),
		Cwd:  inRoot,
	}, nil
}

func splitNUL(b []byte) []string {
	b = bytes.TrimSuffix(b, []byte{0})
	if len(b) == 0 {
		return nil
	}
	parts := bytes.Split(b, []byte{0})
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = string(p)
	}
	return out
}

var observeProcess = procObserve
