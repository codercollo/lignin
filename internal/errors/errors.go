// Package errors defines the canonical error model for the lignin domain.
//
// It provides a strongly-typed error system built around a stable set of
// machine-readable codes.
package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// Code is a machine-readable error classification
type Code string

const (
	CodeNotFound         Code = "NOT_FOUND"
	CodeConflict         Code = "CONFLICT"
	CodeUnauthorized     Code = "UNAUTHORIZED"
	CodeForbidden        Code = "FORBIDDEN"
	CodeInvalidArgument  Code = "INVALID_ARGUMENT"
	CodeInternal         Code = "INTERNAL"
	CodeUnavailable      Code = "UNAVAILABLE"
	CodeDuplicate        Code = "DUPLICATE"
	CodeSignatureInvalid Code = "SIGNATURE_INVALID"
	CodeTokenExpired     Code = "TOKEN_EXPIRED"
	CodeRateLimited      Code = "RATE_LIMITED"
)

// Error is the core domain error type used throughout the app
type Error struct {
	Code    Code
	Message string
	Cause   error
}

// Error implements the standard error interface.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}

	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap exposes the underlying cause to support errors.Is / errors.As
func (e *Error) Unwrap() error {
	return e.Cause
}

// New creates a new domain error without an underlying cause
func New(code Code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// Wrap creates a new domain error with an underlying cause
func Wrap(code Code, message string, cause error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

//---- Constructors ------

// NotFound creates a standardized NOT_FOUND error for a given resource
func NotFound(resource string) *Error {
	return New(CodeNotFound, fmt.Sprintf("%s not found", resource))
}

// Conflict creates a CONFLICT error when a resourse already exists
func Conflict(resource string) *Error {
	return New(CodeConflict, fmt.Sprintf("%s already exists", resource))
}

// Unauthorized creates an UNAUTHORIZED error with a provided reason.
func Unauthorized(reason string) *Error {
	return New(CodeUnauthorized, reason)
}

// Forbidden creates a FORBIDDEN error with a provided reason.
func Forbidden(reason string) *Error {
	return New(CodeForbidden, reason)
}

// InvalidArgument creates an INVALID_ARGUMENT error for a specific field.
func InvalidArgument(field, reason string) *Error {
	return New(CodeInvalidArgument, fmt.Sprintf("invalid %s: %s", field, reason))
}

// Internal wraps an unexpected internal error used for system-level failures.
func Internal(cause error) *Error {
	return Wrap(CodeInternal, "an internal error occurred", cause)
}

// Unavailable wraps errors when a downstream service is temporarily unavailable.
func Unavailable(cause error) *Error {
	return Wrap(CodeUnavailable, "service temporarily unavailable", cause)
}

// Duplicate creates a DUPLICATE error for conflicting unique keys.
func Duplicate(key string) *Error {
	return New(CodeDuplicate, fmt.Sprintf("duplicate entry for key %q", key))
}

// SignatureInvalid indicates that a request signature could not be verified.
func SignatureInvalid() *Error {
	return New(CodeSignatureInvalid, "request signature is invalid")
}

// TokenExpired indicates that an authentication token is no longer valid.
func TokenExpired() *Error {
	return New(CodeTokenExpired, "access token has expired")
}

// RateLimited indicates that the client has exceeded allowed request limits.
func RateLimited() *Error {
	return New(CodeRateLimited, "rate limit exceeded, please back off")
}

//---- Helpers ------

// As attempts to extract a *Error from an error chain.
func As(err error, target **Error) bool {
	return errors.As(err, target)
}

// Is reports whether the error chain contains a domain error with the given Code
func Is(err error, c Code) bool {
	var de *Error
	if !errors.As(err, &de) {
		return false
	}

	return de.Code == c
}

// HTTPStatus maps a domain error to an HTTP status code

func HTTPStatus(err error) int {
	var de *Error
	if !errors.As(err, &de) {
		return http.StatusInternalServerError
	}

	switch de.Code {
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict, CodeDuplicate:
		return http.StatusConflict
	case CodeUnauthorized, CodeTokenExpired, CodeSignatureInvalid:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeInvalidArgument:
		return http.StatusBadRequest
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// UserMessage returns a sanitized message safe for API responses
func UserMessage(err error) string {
	var de *Error
	if !errors.As(err, &de) {
		return "an unexpected error occurred"
	}

	if de.Code == CodeInternal || de.Code == CodeUnavailable {
		return "an internal error occured"
	}

	return de.Message
}
