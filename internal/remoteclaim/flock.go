package remoteclaim

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WithLock acquires an exclusive advisory lock on path+".lock" via
// O_CREAT|O_EXCL|O_WRONLY, retrying up to 50×50ms (from dagctl).
// In-memory databases skip file locking.
func WithLock(path string, fn func() error) error {
	if path == ":memory:" || strings.HasPrefix(path, "file:") && strings.Contains(path, "mode=memory") {
		return fn()
	}
	lockPath := path + ".lock"
	if dir := filepath.Dir(lockPath); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	for i := 0; i < 50; i++ {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			defer os.Remove(lockPath)
			return fn()
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("could not acquire lock on %s", lockPath)
}
