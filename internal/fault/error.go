// Package fault defines the public, secret-free operational error contract.
package fault

import "fmt"

type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
	Status  int            `json:"-"`
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func New(code, message string) *Error {
	return &Error{Code: code, Message: message, Details: map[string]any{}, Status: 1}
}

func Usage(message string) *Error {
	e := New("invocation.invalid", message)
	e.Status = 2
	return e
}

func Interaction(message string) *Error { return New("interaction.required", message) }

func Applied(message, transaction string) *Error {
	e := New("transaction.incomplete", message)
	e.Status = 3
	e.Details["transaction"] = transaction
	e.Details["applied"] = true
	return e
}
