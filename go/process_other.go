//go:build !linux

package localai

import "runtime"

// LocalAI v4.10.0 is reviewed as a Linux binary. Other platforms expose no
// procfs view of another process's command line and environment, and a
// LocalAI inside WSL reaches Windows through a relay process, so attach
// refuses there.
func procObserve(int) (processView, error) {
	return processView{}, refuse(ConditionProcessObservable, "%s cannot read another process's command line and environment", runtime.GOOS)
}

var observeProcess = procObserve
