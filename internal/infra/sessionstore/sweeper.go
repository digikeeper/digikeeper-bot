package sessionstore

import (
	"context"
	"log/slog"
	"time"
)

// expirer is the data operation a Sweeper drives on a schedule. *Repository
// satisfies it; keeping this an interface lets the worker depend only on the one
// query it needs.
type Expirer interface {
	SweepExpired(ctx context.Context) (int64, error)
}

// Sweeper periodically reclaims expired sessions from a store. It is a thin
// scheduler around store.SweepExpired: all SQL lives in the store, all lifecycle
// lives here. It complements the store's lazy per-read expiry by eagerly
// reclaiming rows that are never fetched again.
type Sweeper struct {
	store    Expirer
	interval time.Duration
}

// NewSweeper builds a Sweeper that sweeps store every interval.
func NewSweeper(store Expirer, interval time.Duration) *Sweeper {
	return &Sweeper{store: store, interval: interval}
}

// Run sweeps on every interval tick until ctx is cancelled, then returns. It
// blocks, so callers typically launch it with `go`; cancel ctx to stop it.
func (s *Sweeper) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := s.store.SweepExpired(ctx); err != nil {
				slog.WarnContext(ctx, "session sweep failed", slog.Any("error", err))
			} else if n > 0 {
				slog.DebugContext(ctx, "swept expired sessions", slog.Int64("deleted", n))
			}
		}
	}
}
