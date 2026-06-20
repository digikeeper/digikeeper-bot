package fsm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hishamk/statetrooper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gitrus/digikeeper-bot/pkg/fsm"
)

type state string

const (
	idle    state = "idle"
	running state = "running"
	stopped state = "stopped"
)

const (
	evStart fsm.EventType = "start"
	evStop  fsm.EventType = "stop"
	evPing  fsm.EventType = "ping"
)

// newMachine builds an FSM with the rules idle->running->stopped.
func newMachine() *fsm.FSM[state] {
	st := statetrooper.NewFSM(idle, 10)
	st.AddRule(idle, running)
	st.AddRule(running, stopped)
	return fsm.NewFSM(st)
}

func TestHandleEventTransitions(t *testing.T) {
	m := newMachine()
	m.AddHandler(
		fsm.HandlerKey[state]{State: idle, EventType: evStart},
		func(_ context.Context, _ state, _ fsm.Event) (fsm.HandlerResult[state], error) {
			return fsm.HandlerResult[state]{NextState: running}, nil
		},
	)

	err := m.HandleEvent(t.Context(), fsm.Event{Type: evStart})

	require.NoError(t, err)
	assert.Equal(t, running, m.CurrentState())
}

func TestHandleEventRunsSideEffectAfterTransition(t *testing.T) {
	m := newMachine()

	var sideEffectRan bool
	m.AddHandler(
		fsm.HandlerKey[state]{State: idle, EventType: evStart},
		func(_ context.Context, _ state, _ fsm.Event) (fsm.HandlerResult[state], error) {
			return fsm.HandlerResult[state]{
				NextState:  running,
				SideEffect: func() error { sideEffectRan = true; return nil },
			}, nil
		},
	)

	require.NoError(t, m.HandleEvent(t.Context(), fsm.Event{Type: evStart}))
	assert.True(t, sideEffectRan, "side effect should run after a successful transition")
}

func TestHandleEventNoStateChangeStillRunsSideEffect(t *testing.T) {
	m := newMachine()

	var sideEffectRan bool
	// Handler keeps the same state (idle) - no transition should be attempted,
	// but the side effect must still run.
	m.AddHandler(
		fsm.HandlerKey[state]{State: idle, EventType: evPing},
		func(_ context.Context, s state, _ fsm.Event) (fsm.HandlerResult[state], error) {
			return fsm.HandlerResult[state]{
				NextState:  s,
				SideEffect: func() error { sideEffectRan = true; return nil },
			}, nil
		},
	)

	require.NoError(t, m.HandleEvent(t.Context(), fsm.Event{Type: evPing}))
	assert.Equal(t, idle, m.CurrentState())
	assert.True(t, sideEffectRan)
}

func TestHandleEventMissingHandler(t *testing.T) {
	m := newMachine()

	err := m.HandleEvent(t.Context(), fsm.Event{Type: evStart})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no handler")
}

func TestHandleEventHandlerError(t *testing.T) {
	m := newMachine()

	wantErr := errors.New("boom")
	m.AddHandler(
		fsm.HandlerKey[state]{State: idle, EventType: evStart},
		func(_ context.Context, _ state, _ fsm.Event) (fsm.HandlerResult[state], error) {
			return fsm.HandlerResult[state]{}, wantErr
		},
	)

	err := m.HandleEvent(t.Context(), fsm.Event{Type: evStart})

	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
	assert.Equal(t, idle, m.CurrentState(), "state must not change when the handler fails")
}

func TestHandleEventInvalidTransitionIsRejected(t *testing.T) {
	m := newMachine()

	// idle -> stopped is not an allowed rule, so statetrooper must reject it.
	m.AddHandler(
		fsm.HandlerKey[state]{State: idle, EventType: evStop},
		func(_ context.Context, _ state, _ fsm.Event) (fsm.HandlerResult[state], error) {
			return fsm.HandlerResult[state]{NextState: stopped}, nil
		},
	)

	err := m.HandleEvent(t.Context(), fsm.Event{Type: evStop})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "transition")
	assert.Equal(t, idle, m.CurrentState())
}

func TestHandleEventSideEffectErrorPropagates(t *testing.T) {
	m := newMachine()

	wantErr := errors.New("side effect failed")
	m.AddHandler(
		fsm.HandlerKey[state]{State: idle, EventType: evStart},
		func(_ context.Context, _ state, _ fsm.Event) (fsm.HandlerResult[state], error) {
			return fsm.HandlerResult[state]{
				NextState:  running,
				SideEffect: func() error { return wantErr },
			}, nil
		},
	)

	err := m.HandleEvent(t.Context(), fsm.Event{Type: evStart})

	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
	// The transition already happened before the side effect ran.
	assert.Equal(t, running, m.CurrentState())
}
