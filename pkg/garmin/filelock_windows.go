//go:build windows

package garmin

import (
	"context"
	"os"
	"time"
)

// staleLockAge is how old an O_EXCL sentinel may get before it is presumed
// abandoned by a crashed process and broken.
const staleLockAge = 5 * time.Minute

// acquireFileLock emulates an exclusive lock with an O_EXCL sentinel file
// (flock is not available on Windows without a dependency beyond the standard
// library). The returned function releases the lock.
func acquireFileLock(ctx context.Context, path string) (func(), error) {
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if st, serr := os.Stat(path); serr == nil && time.Since(st.ModTime()) > staleLockAge {
			_ = os.Remove(path)
			continue
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
