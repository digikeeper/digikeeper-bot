// Package fsm adds an event/handler dispatch table on top of
// github.com/hishamk/statetrooper: an Event is routed to the handler for the
// current (state, event-type) pair, which decides the next state, the
// transition metadata and an optional side effect.
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
	Type    EventType
	Payload map[string]string
}

// HandlerResult is what a handler returns. If NextState equals the current
// state no transition is attempted; otherwise Metadata is recorded on it.
type HandlerResult[T comparable] struct {
	NextState  T
	Metadata   map[string]string
	SideEffect func() error
}

// HandlerFunc handles an event for the current state and the metadata recorded
// on the latest transition into it.
type HandlerFunc[T comparable] func(ctx context.Context, state T, metadata map[string]string, event Event) (HandlerResult[T], error)

// HandlerKey identifies a handler by state and event type.
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

// CurrentStateWithMetadata returns the current state and its transition metadata.
func (f *FSM[T]) CurrentStateWithMetadata() (state T, metadata map[string]string) {
	return f.state.CurrentStateWithMetadata()
}

// AddHandler registers a handler for a (state, event-type) pair.
func (f *FSM[T]) AddHandler(handlerKey HandlerKey[T], handler HandlerFunc[T]) {
	f.handlers[handlerKey] = handler
}

// HandleEvent dispatches event to the current state's handler, applies the
// resulting transition (if the state changes) and runs the side effect (if any).
func (f *FSM[T]) HandleEvent(ctx context.Context, event Event) error {
	curState, metadata := f.state.CurrentStateWithMetadata()

	handler, ok := f.handlers[HandlerKey[T]{State: curState, EventType: event.Type}]
	if !ok {
		return fmt.Errorf("fsm: no handler for event %q in state %v", event.Type, curState)
	}

	result, err := handler(ctx, curState, metadata, event)
	if err != nil {
		return fmt.Errorf("fsm: handler for event %q in state %v failed: %w", event.Type, curState, err)
	}

	if result.NextState != curState {
		if _, err := f.state.Transition(result.NextState, result.Metadata); err != nil {
			return fmt.Errorf("fsm: transition from %v to %v failed: %w", curState, result.NextState, err)
		}
	}

	// Runs after the transition, so it is non-atomic: make side effects compensate-driven.
	if result.SideEffect != nil {
		if err := result.SideEffect(); err != nil {
			return fmt.Errorf("fsm: side effect after event %q failed: %w", event.Type, err)
		}
	}

	return nil
}
