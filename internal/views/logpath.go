package views

import (
	"os"
	"path/filepath"
)

// DebugLogPath is where weft writes its debug and sync-failure log.
// Anchored to the user cache dir so a weft launched from inside the
// graph can't deposit weft.log where the next sync's `git add -A`
// would commit it into the notes repo. Falls back to the process cwd
// only when the cache dir is unavailable.
func DebugLogPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		// Cache dir unavailable: fall back to the cwd, reopening the
		// pollution risk this function otherwise prevents.
		return "weft.log"
	}
	p := filepath.Join(dir, "weft")
	if err := os.MkdirAll(p, 0o755); err != nil {
		// Same fallback, same caveat, if the cache dir exists but the
		// weft subdirectory can't be created.
		return "weft.log"
	}
	return filepath.Join(p, "weft.log")
}
