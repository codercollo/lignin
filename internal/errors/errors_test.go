// Package errors_test contains unit tests for the lignin internal error
// package. It verifies correct construction, wrapping behavior, HTTP mapping,
// and safe external representations of domain errors.
package errors_test

import (
	"fmt"
	"net/http"
	"testing"

	errs "github.com/codercollo/lignin/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNew verifies that a domain error can be created without a cause
// and that all fields are correctly assigned.
func TestNew(t *testing.T) {
	e := errs.New(errs.CodeNotFound, "user not found")
	require.NotNil(t, e)
	assert.Equal(t, errs.CodeNotFound, e.Code)
	assert.Equal(t, "user not found", e.Message)
	assert.Nil(t, e.Cause)
}

// TestWrap verifies that wrapping an underlying error prevents the cause
// and integrates properly with error inspection utilies
func TestWrap(t *testing.T) {
	cause := fmt.Errorf("sql: no rows")
	e := errs.Wrap(errs.CodeInternal, "db error", cause)

	require.NotNil(t, e)
	assert.Equal(t, errs.CodeInternal, e.Code)
	assert.ErrorIs(t, e, cause)
}

// TestError_ErrorString verifies the string representation of errors
// both with and without an underlying cause.
func TestError_ErrorString(t *testing.T) {
	t.Run("without_cause", func(t *testing.T) {
		e := errs.New(errs.CodeForbidden, "access denied")
		assert.Equal(t, "[FORBIDDEN] access denied", e.Error())
	})

	t.Run("with_cause", func(t *testing.T) {
		e := errs.Wrap(errs.CodeInternal, "db error", fmt.Errorf("sql: no rows"))
		assert.Contains(t, e.Error(), "[INTERNAL]")
		assert.Contains(t, e.Error(), "sql: no rows")
	})
}

// TestHTTPStatus verifies that each domain error code is mapped to the correct
// HTTP status code, including fallback behavior for unknown errors.
func TestHTTPStatus(t *testing.T) {
	cases := []struct {
		err      error
		expected int
	}{
		{errs.NotFound("transaction"), http.StatusNotFound},
		{errs.Conflict("idempotency key"), http.StatusConflict},
		{errs.Duplicate("tx_ref"), http.StatusConflict},
		{errs.Unauthorized("bad token"), http.StatusUnauthorized},
		{errs.TokenExpired(), http.StatusUnauthorized},
		{errs.SignatureInvalid(), http.StatusUnauthorized},
		{errs.Forbidden("no access"), http.StatusForbidden},
		{errs.InvalidArgument("amount", "must be positive"), http.StatusBadRequest},
		{errs.RateLimited(), http.StatusTooManyRequests},
		{errs.Unavailable(fmt.Errorf("redis down")), http.StatusServiceUnavailable},
		{errs.Internal(fmt.Errorf("nil ptr")), http.StatusInternalServerError},
		{fmt.Errorf("raw error"), http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			assert.Equal(t, tc.expected, errs.HTTPStatus(tc.err))
		})
	}
}

// TestIs verifies classification by error Code across an error chain.
func TestIs(t *testing.T) {
	e := errs.NotFound("user")
	assert.True(t, errs.Is(e, errs.CodeNotFound))
	assert.False(t, errs.Is(e, errs.CodeForbidden))
	assert.False(t, errs.Is(fmt.Errorf("plain"), errs.CodeNotFound))
}

// TestAs verifies extraction of domain errors from wrapped error chains.
func TestAs(t *testing.T) {
	e := errs.NotFound("payments")
	wrapped := fmt.Errorf("handler: %w", e)

	var target *errs.Error
	assert.True(t, errs.As(wrapped, &target))
	assert.Equal(t, errs.CodeNotFound, target.Code)
}

// TestUserMessage verifies that user-facing messages are safe,
// properly exposed, and that internal errors are sanitized.
func TestUserMessage(t *testing.T) {
	t.Run("internal_scrubbed", func(t *testing.T) {
		e := errs.Internal(fmt.Errorf("secret db details"))
		assert.Equal(t, "an internal error occurred", errs.UserMessage(e))
	})

	t.Run("unavailable_scrubbed", func(t *testing.T) {
		e := errs.Unavailable(fmt.Errorf("redis conn refused"))
		assert.Equal(t, "an internal error occurred", errs.UserMessage(e))
	})

	t.Run("user_facing_passes_through", func(t *testing.T) {
		e := errs.NotFound("transaction")
		assert.Equal(t, "transaction not found", errs.UserMessage(e))
	})

	t.Run("raw_error_scrubbed", func(t *testing.T) {
		assert.Equal(t, "an unexpected error occurred", errs.UserMessage(fmt.Errorf("raw")))
	})
}

// TestConvenienceConstructors verifies all helper constructors produce
// correctly coded domain errors.
func TestConvenienceConstructors(t *testing.T) {
	assert.Equal(t, errs.CodeNotFound, errs.NotFound("x").Code)
	assert.Equal(t, errs.CodeConflict, errs.Conflict("x").Code)
	assert.Equal(t, errs.CodeUnauthorized, errs.Unauthorized("x").Code)
	assert.Equal(t, errs.CodeForbidden, errs.Forbidden("x").Code)
	assert.Equal(t, errs.CodeInvalidArgument, errs.InvalidArgument("f", "r").Code)
	assert.Equal(t, errs.CodeInternal, errs.Internal(fmt.Errorf("x")).Code)
	assert.Equal(t, errs.CodeUnavailable, errs.Unavailable(fmt.Errorf("x")).Code)
	assert.Equal(t, errs.CodeDuplicate, errs.Duplicate("k").Code)
	assert.Equal(t, errs.CodeSignatureInvalid, errs.SignatureInvalid().Code)
	assert.Equal(t, errs.CodeTokenExpired, errs.TokenExpired().Code)
	assert.Equal(t, errs.CodeRateLimited, errs.RateLimited().Code)
}
