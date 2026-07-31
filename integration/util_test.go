//go:build integration

package integration

import "time"

func timeNow() time.Time { return time.Now() }

func ptrOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}
