// Package lignin_test contains integration-style unit tests for the lignin
// configuration loader.
package lignin_test

import (
	"testing"
	"time"

	"github.com/codercollo/lignin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// baseEnv returns a minimal valid environment map required for lignin.Load to succeed.
func baseEnv(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"APP_ENV":               "test",
		"DATABASE_DSN":          "postgres://user:pass@localhost:5432/lignin_test",
		"MPESA_CONSUMER_KEY":    "key",
		"MPESA_CONSUMER_SECRET": "secret",
		"MPESA_TOKEN_URL":       "https://sandbox.safaricom.co.ke/oauth/v1/generate",
		"JWT_SECRET":            "supersecretjwtsecret32byteslong!!",
		"CALLBACK_SIG_SECRET":   "callbacksecret",
	}
}

// setEnv applies a set of environment variables for a test case.
func setEnv(t *testing.T, pairs map[string]string) {
	t.Helper()
	for k, v := range pairs {
		t.Setenv(k, v)
	}
}

// TestLoad_MinimalValid verifies that a fully valid minimal environment
// loads without error and produces expected default config values
func TestLoad_MinimalValid(t *testing.T) {
	setEnv(t, baseEnv(t))

	cfg, err := lignin.Load()
	require.NoError(t, err)

	assert.Equal(t, "test", cfg.App.Env)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 10*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, 25, cfg.Database.MaxOpenConns)
	assert.Equal(t, 60*time.Second, cfg.Auth.TokenBufferDuration)
}

// TestLoad_ServerAddr verifies that SERVER_HOST and SERVER_PORT are correctly
// combined into a single address string.
func TestLoad_ServerAddr(t *testing.T) {
	e := baseEnv(t)
	e["SERVER_HOST"] = "127.0.0.1"
	e["SERVER_PORT"] = "9000"
	setEnv(t, e)

	cfg, err := lignin.Load()
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:9000", cfg.Server.Addr())
}

// TestLoad_MissingRequired ensures that omitting required environment variables
// results in a configuration error.
func TestLoad_MissingRequired(t *testing.T) {
	required := []string{
		"DATABASE_DSN",
		"MPESA_CONSUMER_KEY",
		"MPESA_CONSUMER_SECRET",
		"MPESA_TOKEN_URL",
		"JWT_SECRET",
		"CALLBACK_SIG_SECRET",
	}

	for _, key := range required {
		t.Run("missing_"+key, func(t *testing.T) {
			e := baseEnv(t)
			delete(e, key)
			setEnv(t, e)

			_, err := lignin.Load()
			assert.Error(t, err, "expected error when %s is missing", key)
		})
	}
}

// TestLoad_InvalidAppEnv verifies that unsupported APP_ENV values are rejected.
func TestLoad_InvalidAppEnv(t *testing.T) {
	e := baseEnv(t)
	e["APP_ENV"] = "banana"
	setEnv(t, e)

	_, err := lignin.Load()
	assert.ErrorContains(t, err, "APP_ENV")
}

// TestLoad_InvalidPort ensures that invalid server port values are rejected
// during configuration parsing.
func TestLoad_InvalidPort(t *testing.T) {
	e := baseEnv(t)
	e["SERVER_PORT"] = "99999"
	setEnv(t, e)

	_, err := lignin.Load()
	assert.ErrorContains(t, err, "SERVER_PORT")
}

// TestLoad_MaxOpenConnsLessThanMaxIdle ensures database connection pool
// constraints are validated correctly.
func TestLoad_MaxOpenConnsLessThanMaxIdle(t *testing.T) {
	e := baseEnv(t)
	e["DB_MAX_OPEN_CONNS"] = "3"
	e["DB_MAX_IDLE_CONNS"] = "10"
	setEnv(t, e)

	_, err := lignin.Load()
	assert.ErrorContains(t, err, "DB_MAX_OPEN_CONNS")
}

// TestLoad_IsDevelopment verifies that development environment is correctly
// detected and corresponding helpers behave as expected.
func TestLoad_IsDevelopment(t *testing.T) {
	e := baseEnv(t)
	e["APP_ENV"] = "development"
	setEnv(t, e)

	cfg, err := lignin.Load()
	require.NoError(t, err)
	assert.True(t, cfg.IsDevelopment())
	assert.False(t, cfg.IsTest())
}

// TestLoad_IsTest verifies that test environemnt detection works correctly
func TestLoad_IsTest(t *testing.T) {
	setEnv(t, baseEnv(t))

	cfg, err := lignin.Load()
	require.NoError(t, err)
	assert.True(t, cfg.IsTest())
	assert.False(t, cfg.IsDevelopment())
}
