// Package sessionstore holds persistent adapters for the session port defined in pkg/sessionmanager.
package sessionstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gitrus/digikeeper-bot/internal/infra/sqlitedb"
	session "github.com/gitrus/digikeeper-bot/pkg/sessionmanager"
)

// Repository is a persistent session.UserSessionManager backed by SQLite. It is
// a passive store: it guarantees correctness lazily (expired rows are filtered
// on read) and owns no goroutines. Eager reclamation of expired rows is the job
// of a Sweeper layered on top (see SweepExpired).
//
// Sessions are keyed by the (chat, user, thread) triple, so the same user has
// independent sessions across chats and forum topics.
type Repository[S session.UserSession] struct {
	db                *sql.DB
	userSessionFabric session.NewUserSession[S]
	ttl               time.Duration

	now func() time.Time
}

// New builds a session repository over a ready-to-use *sql.DB (see
// internal/infra/sqlitedb.Open, which opens, configures, and migrates the pool).
// The caller retains ownership of db and must close it.
//
// A ttl of 0 disables expiry entirely (no expires_at written, nothing to sweep).
func New[S session.UserSession](
	db *sql.DB, usf session.NewUserSession[S], ttl time.Duration,
) (*Repository[S], error) {
	return &Repository[S]{
		db:                db,
		userSessionFabric: usf,
		ttl:               ttl,
		now:               time.Now,
	}, nil
}

// GetOrCreate returns the existing session for key or atomically inserts a
// fresh session. It never replaces an existing session, including when two
// updates for a new key arrive concurrently.
func (m *Repository[S]) GetOrCreate(ctx context.Context, key session.SessionKey) (S, error) {
	state, err := m.Fetch(ctx, key)
	if err == nil {
		return state, nil
	}
	if !errors.Is(err, session.ErrSessionNotFound) {
		var zero S
		return zero, err
	}

	var zero S
	newSession, err := m.userSessionFabric(key)
	if err != nil {
		return zero, err
	}
	data, err := EncodeSession(newSession)
	if err != nil {
		return zero, err
	}

	now := m.now().Unix()
	row := sqlitedb.UserSessionRow{
		ChatID:    key.ChatID,
		UserID:    key.UserID,
		ThreadID:  key.ThreadID,
		Data:      data,
		Version:   newSession.GetVersion(),
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: m.expiryAt(now),
	}
	res, err := m.db.ExecContext(ctx, `
		INSERT INTO user_sessions (tg_chat_id, tg_user_id, tg_thread_id, data, version, created_at, updated_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(tg_chat_id, tg_user_id, tg_thread_id) DO NOTHING`,
		row.ChatID, row.UserID, row.ThreadID, row.Data, row.Version, row.CreatedAt, row.UpdatedAt, row.ExpiresAt,
	)
	if err != nil {
		return zero, session.ErrSessionManagement{Reason: fmt.Sprintf("create session for %s: %v", key, err)}
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return zero, session.ErrSessionManagement{Reason: fmt.Sprintf("rows affected for %s: %v", key, err)}
	}
	if affected == 1 {
		return newSession, nil
	}

	return m.Fetch(ctx, key)
}

// InitSession is retained for backwards compatibility. New callers should use
// GetOrCreate, which does not replace an existing session.
func (m *Repository[S]) InitSession(ctx context.Context, key session.SessionKey) (S, error) {
	return m.GetOrCreate(ctx, key)
}

// Fetch returns the stored session for key. Expired rows are treated as missing
// (and best-effort deleted); a live row has its TTL window slid forward.
func (m *Repository[S]) Fetch(ctx context.Context, key session.SessionKey) (S, error) {
	var zero S

	var row sqlitedb.UserSessionRow
	err := m.db.QueryRowContext(ctx,
		`SELECT data, expires_at FROM user_sessions WHERE tg_chat_id = ? AND tg_user_id = ? AND tg_thread_id = ?`,
		key.ChatID, key.UserID, key.ThreadID,
	).Scan(&row.Data, &row.ExpiresAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return zero, fmt.Errorf("%w for %s", session.ErrSessionNotFound, key)
	case err != nil:
		return zero, session.ErrSessionManagement{Reason: fmt.Sprintf("fetch user session for %s: %v", key, err)}
	}

	now := m.now().Unix()
	if row.ExpiresAt.Valid && row.ExpiresAt.Int64 < now {
		m.deleteExpired(ctx, key)
		return zero, fmt.Errorf("%w for %s", session.ErrSessionNotFound, key)
	}

	userSession, err := DecodeSession[S](row.Data)
	if err != nil {
		return zero, err
	}

	if m.ttl > 0 {
		if _, err := m.db.ExecContext(ctx,
			`UPDATE user_sessions SET expires_at = ? WHERE tg_chat_id = ? AND tg_user_id = ? AND tg_thread_id = ?`,
			m.expiryAt(now), key.ChatID, key.UserID, key.ThreadID,
		); err != nil {
			return zero, session.ErrSessionManagement{Reason: fmt.Sprintf("slide ttl for %s: %v", key, err)}
		}
	}

	return userSession, nil
}

// Set applies an optimistic update: the row is changed only when its stored
// version still equals prevVersion. The TTL window is slid forward on success.
func (m *Repository[S]) Set(
	ctx context.Context, key session.SessionKey, newSession S, prevVersion int,
) (S, error) {
	var zero S

	data, err := EncodeSession(newSession)
	if err != nil {
		return zero, err
	}

	now := m.now().Unix()
	res, err := m.db.ExecContext(ctx, `
		UPDATE user_sessions
		SET data = ?, version = ?, updated_at = ?, expires_at = ?
		WHERE tg_chat_id = ? AND tg_user_id = ? AND tg_thread_id = ? AND version = ?`,
		data, newSession.GetVersion(), now, m.expiryAt(now), key.ChatID, key.UserID, key.ThreadID, prevVersion,
	)
	if err != nil {
		return zero, session.ErrSessionManagement{Reason: fmt.Sprintf("set user session for %s: %v", key, err)}
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return zero, session.ErrSessionManagement{Reason: fmt.Sprintf("rows affected for %s: %v", key, err)}
	}
	if affected == 1 {
		return newSession, nil
	}

	// No row updated: distinguish a missing session from a version mismatch so
	// the error matches the in-memory manager's semantics.
	var current sqlitedb.UserSessionRow
	err = m.db.QueryRowContext(ctx,
		`SELECT data, expires_at FROM user_sessions WHERE tg_chat_id = ? AND tg_user_id = ? AND tg_thread_id = ?`,
		key.ChatID, key.UserID, key.ThreadID,
	).Scan(&current.Data, &current.ExpiresAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return zero, session.ErrSessionManagement{Reason: "session not found"}
	case err != nil:
		return zero, session.ErrSessionManagement{Reason: fmt.Sprintf("inspect user session for %s: %v", key, err)}
	}

	if current.ExpiresAt.Valid && current.ExpiresAt.Int64 < now {
		m.deleteExpired(ctx, key)
		return zero, session.ErrSessionManagement{Reason: "session not found"}
	}

	oldSession, err := DecodeSession[S](current.Data)
	if err != nil {
		return zero, err
	}
	return oldSession, session.ErrSessionManagement{Reason: "version mismatch"}
}

// DropActive removes the session for key. It is idempotent.
func (m *Repository[S]) DropActive(ctx context.Context, key session.SessionKey) error {
	if _, err := m.db.ExecContext(ctx,
		`DELETE FROM user_sessions WHERE tg_chat_id = ? AND tg_user_id = ? AND tg_thread_id = ?`,
		key.ChatID, key.UserID, key.ThreadID,
	); err != nil {
		return session.ErrSessionManagement{Reason: fmt.Sprintf("drop user session for %s: %v", key, err)}
	}
	return nil
}

// SweepExpired deletes all rows whose expires_at is in the past and returns the
// number removed. It is the data operation a Sweeper drives on a schedule: the
// SQL stays here in the store while the scheduling lives in the worker.
func (m *Repository[S]) SweepExpired(ctx context.Context) (int64, error) {
	res, err := m.db.ExecContext(ctx,
		`DELETE FROM user_sessions WHERE expires_at IS NOT NULL AND expires_at < ?`, m.now().Unix(),
	)
	if err != nil {
		return 0, session.ErrSessionManagement{Reason: fmt.Sprintf("sweep expired sessions: %v", err)}
	}
	return res.RowsAffected()
}

// expiryAt returns the expires_at value for a write happening at unix second
// now. It is NULL when TTL is disabled.
func (m *Repository[S]) expiryAt(now int64) sql.NullInt64 {
	if m.ttl <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: now + int64(m.ttl.Seconds()), Valid: true}
}

// deleteExpired removes a single expired row on a best-effort basis (lazy
// expiry); a failure here is non-fatal and only logged.
func (m *Repository[S]) deleteExpired(ctx context.Context, key session.SessionKey) {
	if _, err := m.db.ExecContext(ctx,
		`DELETE FROM user_sessions WHERE tg_chat_id = ? AND tg_user_id = ? AND tg_thread_id = ?`,
		key.ChatID, key.UserID, key.ThreadID,
	); err != nil {
		slog.DebugContext(ctx, "best-effort delete of expired session failed",
			slog.String("session_key", key.String()), slog.Any("error", err))
	}
}
