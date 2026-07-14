# Using the FSM Package in Digikeeper Bot

This document explains how to use the Finite State Machine (FSM) package in the Digikeeper Bot to implement multi-step interactions.

## Overview

The FSM package provides a simple implementation of a Finite State Machine that can be used to manage the state of multi-step interactions with users. It allows you to define states, events, and transitions between states, as well as actions to be executed when transitions occur.

## Example: Note-Taking Flow

The `internal/cmd_handler/note_fsm.go` file provides an example of how to use the FSM package to implement a multi-step note-taking interaction. The flow is as follows:

1. User sends `/addnote` command
2. Bot asks for a title
3. User enters a title
4. Bot asks for a body
5. User enters a body
6. Bot shows a summary and asks for confirmation
7. User confirms or cancels
8. Bot saves the note or discards it

## How to Use the FSM Package

### 1. Define States and Events

```go
// Define states for your flow
const (
    StateIdle       fsm.State = "idle"
    StateWaitTitle  fsm.State = "wait_title"
    StateWaitBody   fsm.State = "wait_body"
    StateConfirming fsm.State = "confirming"
    StateCompleted  fsm.State = "completed"
)

// Define events that can trigger state transitions
const (
    EventStartNote    fsm.Event = "start_note"
    EventTitleEntered fsm.Event = "title_entered"
    EventBodyEntered  fsm.Event = "body_entered"
    EventConfirm      fsm.Event = "confirm"
    EventCancel       fsm.Event = "cancel"
    EventReset        fsm.Event = "reset"
)
```

### 2. Create a Data Structure for Your Flow

```go
// Define a data structure to hold the data for your flow
type Note struct {
    Title string
    Body  string
}
```

### 3. Create a Manager for Your Flow

```go
// Create a manager for your flow
type NoteManager struct {
    noteFSM *fsm.FSM
    note    *Note
    usm     UserStateManager
}

// Create a new manager
func NewNoteManager(usm UserStateManager) *NoteManager {
    nm := &NoteManager{
        noteFSM: fsm.NewFSM(StateIdle),
        note:    &Note{},
        usm:     usm,
    }
    
    // Define transition actions
    // ...
    
    // Add transitions
    nm.noteFSM.AddTransition(StateIdle, EventStartNote, StateWaitTitle, resetNote)
    nm.noteFSM.AddTransition(StateWaitTitle, EventTitleEntered, StateWaitBody, saveTitle)
    // ...
    
    return nm
}
```

### 4. Create Command Handlers

```go
// Create a handler for the command that starts the flow
func HandleAddNote(usm UserStateManager) th.Handler {
    nm := NewNoteManager(usm)
    
    return func(ctx *th.Context, update telego.Update) error {
        // Start the flow
        err := nm.noteFSM.Trigger(EventStartNote, nil)
        // ...
        
        // Store the current state
        state := string(nm.noteFSM.CurrentState())
        _, err = usm.Set(userID, state)
        // ...
        
        // Send a message to the user
        // ...
        
        return nil
    }
}

// Create a handler for messages that are part of the flow
func HandleMessage(usm UserStateManager) th.Handler {
    nm := NewNoteManager(usm)
    
    return func(ctx *th.Context, update telego.Update) error {
        // Get the current state
        // ...
        
        // Handle the message based on the current state
        switch fsm.State(state) {
        case StateWaitTitle:
            // Handle title input
            // ...
        case StateWaitBody:
            // Handle body input
            // ...
        case StateConfirming:
            // Handle confirmation
            // ...
        }
        
        // If we get here, the message is not part of the flow
        return ctx.Next(update)
    }
}
```

### 5. Register the Handlers

```go
// Register the command handler
cmdHandlerGroup.RegisterCommand("addnote", cmdh.HandleAddNote(usm), "Add a new note using FSM")

// Register the message handler
bh.Handle(cmdh.HandleMessage(usm), th.AnyMessage())
```

## Integration with User State Management

The FSM package works well with the existing user state management system. The FSM state is stored as a string in the user's state, and the FSM is reset to the user's state when a message is received.

## Next Steps

To fully integrate the FSM package into your bot, you should:

1. Implement a proper state storage mechanism that can store complex state objects, not just strings
2. Update the UserStateManager interface to be consistent across the codebase
3. Add more flows using the FSM package, such as search, edit, delete, etc.

## Conclusion

The FSM package provides a powerful way to implement multi-step interactions in your bot. By defining states, events, and transitions, you can create complex flows that are easy to understand and maintain.