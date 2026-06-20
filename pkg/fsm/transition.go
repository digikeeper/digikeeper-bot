// Package fsm provides a thin, generic event-driven layer on top of
// github.com/hishamk/statetrooper.
//
// statetrooper manages the allowed transitions between states and their
// history. This package adds an event/handler dispatch table on top: an
// incoming Event is routed to the handler registered for the current
// (state, event-type) pair, and the handler decides the next state, any data
// update and an optional side effect to run after the transition succeeds.
package fsm

import (
	"context"
	"fmt"

	"github.com/hishamk/statetrooper"
)

// EventType identifies the kind of event being handled.
type EventType string

const (
	Internal   EventType = "INTERNAL"
	Transition EventType = "TRANSITION"
)

// Event is a single input to the FSM.
type Event struct {
	Type EventType
	// Payload is passed through to statetrooper as transition metadata.
	Payload map[string]string
}

// HandlerResult is what a handler returns: the state to move to, an optional
// updated data payload and an optional side effect to run after the transition
// succeeds.
type HandlerResult[T comparable] struct {
	NextState  T
	NewData    any          // The updated state data, available to the side effect.
	SideEffect func() error // Optional: an action to perform *after* a successful transition.
}

// HandlerFunc handles an event for a given current state.
type HandlerFunc[T comparable] func(ctx context.Context, state T, event Event) (HandlerResult[T], error)

// HandlerKey identifies a handler by the state it applies to and the event type
// it reacts to.
type HandlerKey[T comparable] struct {
	State     T
	EventType EventType
}

// FSM wraps a statetrooper FSM with an event/handler dispatch table.
type FSM[T comparable] struct {
	state    *statetrooper.FSM[T]
	handlers map[HandlerKey[T]]HandlerFunc[T]
}

// NewFSM builds an FSM around an already-configured statetrooper FSM.
func NewFSM[T comparable](stateFSM *statetrooper.FSM[T]) *FSM[T] {
	return &FSM[T]{
		state:    stateFSM,
		handlers: make(map[HandlerKey[T]]HandlerFunc[T]),
	}
}

// CurrentState returns the current state of the underlying machine.
func (f *FSM[T]) CurrentState() T {
	return f.state.CurrentState()
}

// AddHandler registers a handler for a (state, event-type) pair.
func (f *FSM[T]) AddHandler(handlerKey HandlerKey[T], handler HandlerFunc[T]) {
	f.handlers[handlerKey] = handler
}

// HandleEvent dispatches event to the handler registered for the current state,
// applies the resulting transition (if the state changes) and runs the side
// effect (if any).
func (f *FSM[T]) HandleEvent(ctx context.Context, event Event) error {
	curState := f.state.CurrentState()

	handler, ok := f.handlers[HandlerKey[T]{State: curState, EventType: event.Type}]
	if !ok {
		return fmt.Errorf("fsm: no handler for event %q in state %v", event.Type, curState)
	}

	result, err := handler(ctx, curState, event)
	if err != nil {
		return fmt.Errorf("fsm: handler for event %q in state %v failed: %w", event.Type, curState, err)
	}

	if result.NextState != curState {
		if _, err := f.state.Transition(result.NextState, event.Payload); err != nil {
			return fmt.Errorf("fsm: transition from %v to %v failed: %w", curState, result.NextState, err)
		}
	}

	// The transition (if any) has succeeded; now run the side effect. Note the
	// inherent non-atomicity: the state has already changed, so a failing side
	// effect leaves the machine in the new state. Callers that need atomicity
	// should compensate via a follow-up event.
	if result.SideEffect != nil {
		if err := result.SideEffect(); err != nil {
			return fmt.Errorf("fsm: side effect after event %q failed: %w", event.Type, err)
		}
	}

	return nil
}
