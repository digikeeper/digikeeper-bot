package sessionmanager_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/gitrus/digikeeper-bot/pkg/sessionmanager"
)

type MockSession struct {
	mock.Mock
	State   string
	version int
}

func (s *MockSession) GetVersion() int {
	return s.version
}

func NewMockSession(_ sessionmanager.SessionKey) (*MockSession, error) {
	return &MockSession{}, nil
}

func TestUserSessionManagerInMem_interfact(t *testing.T) {
	manager := sessionmanager.NewUserSessionManagerInMem[*MockSession](NewMockSession)
	assert.NotNil(t, manager)

	// act
	_, ok := any(manager).(sessionmanager.UserSessionManager[*MockSession])
	// assert
	assert.True(t, ok, "manager should implement UserSessionManager interface")
}

func TestUserSessionManagerInMem_FetchSet(t *testing.T) {
	ctx := context.Background()
	manager := sessionmanager.NewUserSessionManagerInMem[*MockSession](NewMockSession)
	assert.NotNil(t, manager)
	key := sessionmanager.SessionKey{ChatID: 10, UserID: 123}
	state, err := manager.InitSession(ctx, key)
	assert.NoError(t, err)
	assert.Equal(t, &MockSession{}, state)

	// act
	fetchedState, err := manager.Fetch(ctx, key)

	// assert
	assert.NoError(t, err)
	assert.Equal(t, state, fetchedState)

	// act
	newSession := &MockSession{State: "action", version: 1}
	updatedState, err := manager.Set(ctx, key, newSession, 0)

	// assert
	assert.NoError(t, err)
	assert.Equal(t, newSession, updatedState)

	// act
	fetchedState, err = manager.Fetch(ctx, key)

	// assert
	assert.NoError(t, err)
	assert.Equal(t, newSession, fetchedState)
}

func TestUserSessionManagerInMem_DropActive(t *testing.T) {
	ctx := context.Background()
	manager := sessionmanager.NewUserSessionManagerInMem[*MockSession](NewMockSession)
	assert.NotNil(t, manager)
	key := sessionmanager.SessionKey{ChatID: 10, UserID: 123}
	state, err := manager.InitSession(ctx, key)
	assert.NoError(t, err)
	assert.Equal(t, &MockSession{}, state)

	// act
	newSession := &MockSession{State: "action", version: 1}
	updatedState, err := manager.Set(ctx, key, newSession, 0)

	// assert
	assert.NoError(t, err)
	assert.Equal(t, newSession, updatedState)

	// act
	assert.NoError(t, manager.DropActive(ctx, key))

	// assert
	_, err = manager.Fetch(ctx, key)
	assert.ErrorIs(t, err, sessionmanager.ErrSessionNotFound)
}

func TestUserSessionManagerInMem_FetchEmpty(t *testing.T) {
	ctx := context.Background()
	manager := sessionmanager.NewUserSessionManagerInMem[*MockSession](NewMockSession)
	assert.NotNil(t, manager)

	stored := sessionmanager.SessionKey{ChatID: 10, UserID: 455}
	_, err := manager.Set(ctx, stored, &MockSession{}, 1)
	assert.Error(t, err)

	// act
	missing := sessionmanager.SessionKey{ChatID: 10, UserID: 456}
	_, err = manager.Fetch(ctx, missing)

	// assert
	assert.ErrorIs(t, err, sessionmanager.ErrSessionNotFound)
}

func TestUserSessionManagerInMem_SetVersionMismatch(t *testing.T) {
	ctx := context.Background()
	manager := sessionmanager.NewUserSessionManagerInMem[*MockSession](NewMockSession)
	assert.NotNil(t, manager)
	key := sessionmanager.SessionKey{ChatID: 10, UserID: 789}
	state, err := manager.InitSession(ctx, key)
	assert.NoError(t, err)
	assert.Equal(t, &MockSession{}, state)

	// act
	newSession := &MockSession{State: "action", version: 2}
	updatedState, err := manager.Set(ctx, key, newSession, 1)

	// assert
	assert.Error(t, err)
	assert.Equal(t, sessionmanager.ErrSessionManagement{Reason: "version mismatch"}, err)
	assert.Equal(t, state, updatedState)

	// verify stored session remains unchanged
	fetchedState, err := manager.Fetch(ctx, key)
	assert.NoError(t, err)
	assert.Equal(t, state, fetchedState)
}

// TestUserSessionManagerInMem_PerChatIsolation verifies that the same user has
// independent sessions across different chats/topics.
func TestUserSessionManagerInMem_PerChatIsolation(t *testing.T) {
	ctx := context.Background()
	manager := sessionmanager.NewUserSessionManagerInMem[*MockSession](NewMockSession)

	chatA := sessionmanager.SessionKey{ChatID: 1, UserID: 100}
	chatB := sessionmanager.SessionKey{ChatID: 2, UserID: 100}

	_, err := manager.InitSession(ctx, chatA)
	assert.NoError(t, err)

	// The same user in another chat has no session yet.
	_, err = manager.Fetch(ctx, chatB)
	assert.ErrorIs(t, err, sessionmanager.ErrSessionNotFound)
}
