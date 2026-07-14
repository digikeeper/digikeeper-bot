package sessionstore

import (
	"encoding/json"
	"fmt"

	session "github.com/gitrus/digikeeper-bot/pkg/sessionmanager"
)

// EncodeSession serializes a domain session to its stored JSON representation.
func EncodeSession[S session.UserSession](s S) (string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return "", session.ErrSessionManagement{Reason: fmt.Sprintf("marshal session: %v", err)}
	}
	return string(data), nil
}

// DecodeSession parses a stored JSON representation back into a domain session.
func DecodeSession[S session.UserSession](data string) (S, error) {
	var s S
	if err := json.Unmarshal([]byte(data), &s); err != nil {
		var zero S
		return zero, session.ErrSessionManagement{Reason: fmt.Sprintf("unmarshal session: %v", err)}
	}
	return s, nil
}
