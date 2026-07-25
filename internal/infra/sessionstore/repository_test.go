package sessionstore

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gitrus/digikeeper-bot/internal/infra/sqlitedb"
	session "github.com/gitrus/digikeeper-bot/pkg/sessionmanager"
)

// stateAdd is an arbitrary session state used across the update tests.
const stateAdd = "add"

// compile-time assertion that the SQLite manager satisfies the port.
var _ session.UserSessionManager[*session.SimpleUserSession] = (*Repository[*session.SimpleUserSession])(nil)

// newTestManager returns a manager over a fresh in-memory database. A single
// connection (enforced by the constructor) keeps the ":memory:" database alive
// across queries for the lifetime of the manager.
func newTestManager(t *testing.T, ttl time.Duration) *Repository[*session.SimpleUserSession] {
	t.Helper()

	db, err := sqlitedb.Open(":memory:")
	require.NoError(t, err)

	m, err := New(db, session.NewSimpleUserSession, ttl)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, sqlitedb.Close(db))
	})

	return m
}

// withClock overrides the manager clock for deterministic TTL tests. The TTL in
// these tests is large enough that the background sweeper never ticks, so the
// override is only ever read from the test goroutine.
func withClock(m *Repository[*session.SimpleUserSession], at *time.Time) {
	m.now = func() time.Time { return *at }
}

func TestRepository_InitFetchSetFetch(t *testing.T) {
	ctx := t.Context()
	m := newTestManager(t, 0)
	key := session.SessionKey{ChatID: 99, UserID: 123}

	state, err := m.InitSession(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, &session.SimpleUserSession{SessionKey: key, State: "", Version: 1}, state)

	fetched, err := m.Fetch(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, state, fetched)

	updated := &session.SimpleUserSession{SessionKey: key, State: stateAdd, Version: 2}
	got, err := m.Set(ctx, key, updated, 1)
	require.NoError(t, err)
	assert.Equal(t, updated, got)

	fetched, err = m.Fetch(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, updated, fetched)
}

func TestRepository_DropActive(t *testing.T) {
	ctx := t.Context()
	m := newTestManager(t, 0)
	key := session.SessionKey{ChatID: 99, UserID: 123}

	_, err := m.InitSession(ctx, key)
	require.NoError(t, err)

	require.NoError(t, m.DropActive(ctx, key))

	_, err = m.Fetch(ctx, key)
	assert.ErrorIs(t, err, session.ErrSessionNotFound)

	// DropActive is idempotent.
	require.NoError(t, m.DropActive(ctx, key))
}

func TestRepository_FetchUnknown(t *testing.T) {
	ctx := t.Context()
	m := newTestManager(t, 0)
	key := session.SessionKey{ChatID: 99, UserID: 456}

	_, err := m.Fetch(ctx, key)
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
}

func TestRepository_GetOrCreatePreservesExistingSession(t *testing.T) {
	ctx := t.Context()
	m := newTestManager(t, 0)
	key := session.SessionKey{ChatID: 99, UserID: 456}

	_, err := m.GetOrCreate(ctx, key)
	require.NoError(t, err)

	existing := &session.SimpleUserSession{SessionKey: key, State: stateAdd, Version: 2}
	_, err = m.Set(ctx, key, existing, 1)
	require.NoError(t, err)

	got, err := m.GetOrCreate(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, existing, got)
}

func TestRepository_GetOrCreateDoesNotOverwriteAfterFetchError(t *testing.T) {
	ctx := t.Context()
	m := newTestManager(t, 0)
	key := session.SessionKey{ChatID: 99, UserID: 456}
	now := m.now().Unix()

	_, err := m.db.ExecContext(ctx, `
		INSERT INTO user_sessions (tg_chat_id, tg_user_id, tg_thread_id, data, version, created_at, updated_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		key.ChatID, key.UserID, key.ThreadID, "invalid JSON", 1, now, now, nil,
	)
	require.NoError(t, err)

	_, err = m.GetOrCreate(ctx, key)
	require.Error(t, err)

	var data string
	require.NoError(t, m.db.QueryRowContext(ctx,
		`SELECT data FROM user_sessions WHERE tg_chat_id = ? AND tg_user_id = ? AND tg_thread_id = ?`,
		key.ChatID, key.UserID, key.ThreadID,
	).Scan(&data))
	assert.Equal(t, "invalid JSON", data)
}

func TestRepository_SetVersionMismatch(t *testing.T) {
	ctx := t.Context()
	m := newTestManager(t, 0)
	key := session.SessionKey{ChatID: 99, UserID: 789}

	state, err := m.InitSession(ctx, key)
	require.NoError(t, err)

	newSession := &session.SimpleUserSession{SessionKey: key, State: stateAdd, Version: 2}
	got, err := m.Set(ctx, key, newSession, 5)
	assert.Equal(t, session.ErrSessionManagement{Reason: "version mismatch"}, err)
	assert.Equal(t, state, got, "stored session is returned on mismatch")

	fetched, err := m.Fetch(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, state, fetched, "stored session is unchanged")
}

func TestRepository_SetMissing(t *testing.T) {
	ctx := t.Context()
	m := newTestManager(t, 0)
	key := session.SessionKey{ChatID: 99, UserID: 999}

	_, err := m.Set(ctx, key, &session.SimpleUserSession{SessionKey: key, Version: 1}, 0)
	assert.Equal(t, session.ErrSessionManagement{Reason: session.ReasonSessionNotFound}, err)
}

func TestRepository_TTLExpiryLazy(t *testing.T) {
	ctx := t.Context()
	m := newTestManager(t, 100*time.Second)
	now := time.Unix(1000, 0)
	withClock(m, &now)
	key := session.SessionKey{ChatID: 99, UserID: 42}

	_, err := m.InitSession(ctx, key) // expires at 1100
	require.NoError(t, err)

	now = time.Unix(1050, 0)
	_, err = m.Fetch(ctx, key)
	require.NoError(t, err, "still alive before expiry")

	now = time.Unix(1300, 0) // well past any slid expiry
	_, err = m.Fetch(ctx, key)
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
}

func TestRepository_SlidingTTL(t *testing.T) {
	ctx := t.Context()
	m := newTestManager(t, 100*time.Second)
	now := time.Unix(1000, 0)
	withClock(m, &now)
	key := session.SessionKey{ChatID: 99, UserID: 7}

	_, err := m.InitSession(ctx, key) // expires 1100
	require.NoError(t, err)

	now = time.Unix(1090, 0) // before expiry -> Fetch slides expiry to 1190
	_, err = m.Fetch(ctx, key)
	require.NoError(t, err)

	now = time.Unix(1150, 0) // past original 1100 but within slid 1190
	_, err = m.Fetch(ctx, key)
	require.NoError(t, err, "sliding TTL keeps an active session alive")

	// Set also slides the window.
	now = time.Unix(1240, 0) // within previously slid 1250
	_, err = m.Set(ctx, key, &session.SimpleUserSession{SessionKey: key, State: "x", Version: 2}, 1)
	require.NoError(t, err)
}

func TestRepository_Sweeper(t *testing.T) {
	ctx := t.Context()
	m := newTestManager(t, 100*time.Second)
	now := time.Unix(1000, 0)
	withClock(m, &now)

	for _, id := range []int64{1, 2, 3} {
		_, err := m.InitSession(ctx, session.SessionKey{ChatID: 99, UserID: id})
		require.NoError(t, err)
	}

	now = time.Unix(2000, 0) // all expired
	deleted, err := m.SweepExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3), deleted)

	var count int
	require.NoError(t, m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_sessions`).Scan(&count))
	assert.Zero(t, count)
}

// TestRepository_PerChatIsolation verifies the same user has independent
// sessions across different chats.
func TestRepository_PerChatIsolation(t *testing.T) {
	ctx := t.Context()
	m := newTestManager(t, 0)

	chatA := session.SessionKey{ChatID: 1, UserID: 100}
	chatB := session.SessionKey{ChatID: 2, UserID: 100}

	_, err := m.InitSession(ctx, chatA)
	require.NoError(t, err)

	_, err = m.Fetch(ctx, chatB)
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
}

func TestRepository_PersistenceAcrossInstances(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "sessions.db")
	key := session.SessionKey{ChatID: 99, UserID: 555}

	db1, err := sqlitedb.Open(path)
	require.NoError(t, err)
	first, err := New(db1, session.NewSimpleUserSession, 0)
	require.NoError(t, err)

	_, err = first.InitSession(ctx, key)
	require.NoError(t, err)
	saved := &session.SimpleUserSession{SessionKey: key, State: "persisted", Version: 2}
	_, err = first.Set(ctx, key, saved, 1)
	require.NoError(t, err)
	require.NoError(t, sqlitedb.Close(db1))

	db2, err := sqlitedb.Open(path)
	require.NoError(t, err)
	second, err := New(db2, session.NewSimpleUserSession, 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sqlitedb.Close(db2))
	})

	fetched, err := second.Fetch(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, saved, fetched)
}
