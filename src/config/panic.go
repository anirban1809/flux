package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

// WritePanicLog writes a panic report (value + full stack trace) to
// ~/.flux/panic-<timestamp>.log. Call it from a deferred recover() handler.
func WritePanicLog(v any) {
	dir := Cfg.HomeDir
	if dir == "" {
		// config.Load may not have run yet — fall back to raw path.
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "panic: %v\n%s\n", v, debug.Stack())
			return
		}
		dir = filepath.Join(home, ".flux")
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "panic: %v\n%s\n", v, debug.Stack())
		return
	}

	stamp := time.Now().UTC().Format("20060102-150405")
	path := filepath.Join(dir, fmt.Sprintf("panic-%s.log", stamp))

	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "panic: %v\n%s\n", v, debug.Stack())
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "time: %s\npanic: %v\n\nstack trace:\n%s\n",
		time.Now().UTC().Format(time.RFC3339), v, debug.Stack())
	fmt.Fprintf(os.Stderr, "panic captured → %s\n", path)
}
