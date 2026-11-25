//go:build unix

package media

import (
	"context"
	"os/exec"
)

// lowPriorityClass is Windows' way; elsewhere nice and ionice do it (nil: use them).
func lowPriorityClass(ctx context.Context, name string, args ...string) *exec.Cmd { return nil }

// windowsMemory is only read on Windows.
func windowsMemory() int64 { return 0 }

// extraPipes: a child can be given more pipes than stdout and stderr (exec's ExtraFiles).
const extraPipes = true
