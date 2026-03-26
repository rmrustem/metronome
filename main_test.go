package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetEnvStr(t *testing.T) {
	key := "METRONOME_TEST_VAR"
	fallback := "default"

	// Test default value
	os.Unsetenv(key)
	assert.Equal(t, fallback, getEnvStr(key, fallback))

	// Test environment value
	os.Setenv(key, "custom")
	defer os.Unsetenv(key)
	assert.Equal(t, "custom", getEnvStr(key, fallback))
}

func TestWebListenAddr(t *testing.T) {
	key := "METRONOME_WEB_LISTEN"
	fallback := ":8080"

	os.Unsetenv(key)
	assert.Equal(t, fallback, getEnvStr(key, fallback))

	os.Setenv(key, ":9090")
	defer os.Unsetenv(key)
	assert.Equal(t, ":9090", getEnvStr(key, fallback))
}
