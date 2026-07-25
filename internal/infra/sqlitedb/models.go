package sqlitedb

import "database/sql"

// UserSessionRow is the SQL row shape of the user_sessions table. A session is
// keyed by the (chat, user, thread) triple.
type UserSessionRow struct {
	ChatID    int64
	UserID    int64
	ThreadID  int64
	Data      string
	Version   int
	CreatedAt int64
	UpdatedAt int64
	ExpiresAt sql.NullInt64
}
