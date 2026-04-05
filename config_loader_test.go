package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestNewConfigLoader_File(t *testing.T) {
	content := []byte(`
probes:
  - name: "test_probe"
    proto: "http"
    target: "http://localhost:8080"
    interval: 10s
`)
	tmpfile, err := os.CreateTemp("", "config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	os.Setenv("METRONOME_CONFIG_PATH", tmpfile.Name())
	defer os.Unsetenv("METRONOME_CONFIG_PATH")
	loader, err := NewConfigLoader()
	if err != nil {
		t.Fatalf("NewConfigLoader failed: %v", err)
	}
	defer loader.Stop()
	loader.Start()

	select {
	case config := <-loader.Changes():
		if len(config.Probes) != 1 {
			t.Errorf("Expected 1 probe, got %d", len(config.Probes))
		}
		if config.Probes[0].Name != "test_probe" {
			t.Errorf("Expected probe name 'test_probe', got '%s'", config.Probes[0].Name)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for config")
	}
}

func TestNewConfigLoader_File_NotFound(t *testing.T) {
	os.Setenv("METRONOME_CONFIG_PATH", "nonexistent.yaml")
	defer os.Unsetenv("METRONOME_CONFIG_PATH")
	_, err := NewConfigLoader()
	if err == nil {
		t.Fatal("Expected an error for a nonexistent file, but got nil")
	}
}

func TestNewConfigLoader_File_Malformed(t *testing.T) {
	content := []byte(`
probes:
  - name: "test_probe"
    proto: "http"
    target: "http://localhost:8080"
    interval: 10s
malformed
`)
	tmpfile, err := os.CreateTemp("", "config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	os.Setenv("METRONOME_CONFIG_PATH", tmpfile.Name())
	defer os.Unsetenv("METRONOME_CONFIG_PATH")
	_, err = NewConfigLoader()
	if err == nil {
		t.Fatal("Expected an error for a malformed file, but got nil")
	}
}

func TestNewConfigLoader_URL(t *testing.T) {
	content := `
probes:
  - name: "test_probe_url"
    proto: "http"
    target: "http://localhost:9090"
    interval: 15s
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(content))
	}))
	defer server.Close()

	os.Setenv("METRONOME_CONFIG_PATH", "")
	defer os.Unsetenv("METRONOME_CONFIG_PATH")
	os.Setenv("METRONOME_CONFIG_URL", server.URL)
	defer os.Unsetenv("METRONOME_CONFIG_URL")

	loader, err := NewConfigLoader()
	if err != nil {
		t.Fatalf("NewConfigLoader failed: %v", err)
	}
	defer loader.Stop()
	loader.Start()

	select {
	case config := <-loader.Changes():
		if len(config.Probes) != 1 {
			t.Errorf("Expected 1 probe, got %d", len(config.Probes))
		}
		if config.Probes[0].Name != "test_probe_url" {
			t.Errorf("Expected probe name 'test_probe_url', got '%s'", config.Probes[0].Name)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for config")
	}
}

func TestNewConfigLoader_URL_WithAuth(t *testing.T) {
	authHeaderValue := "Bearer my-secret-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != authHeaderValue {
			t.Errorf("Expected Authorization header '%s', got '%s'", authHeaderValue, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`probes: []`))
	}))
	defer server.Close()

	os.Setenv("METRONOME_CONFIG_PATH", "")
	defer os.Unsetenv("METRONOME_CONFIG_PATH")
	os.Setenv("METRONOME_CONFIG_URL", server.URL)
	defer os.Unsetenv("METRONOME_CONFIG_URL")
	os.Setenv("METRONOME_CONFIG_URL_AUTH", authHeaderValue)
	defer os.Unsetenv("METRONOME_CONFIG_URL_AUTH")

	loader, err := NewConfigLoader()
	if err != nil {
		t.Fatalf("NewConfigLoader failed: %v", err)
	}
	defer loader.Stop()
	loader.Start()

	select {
	case <-loader.Changes():
		// config loaded successfully
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for config")
	}
}

func TestNewConfigLoader_File_Reload(t *testing.T) {
	os.Setenv("METRONOME_CONFIG_RELOAD_INTERVAL", "1")
	defer os.Unsetenv("METRONOME_CONFIG_RELOAD_INTERVAL")

	content1 := []byte(`
probes:
  - name: "test_probe"
    proto: "http"
    target: "http://localhost:8080"
`)
	tmpfile, err := os.CreateTemp("", "config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(content1); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	os.Setenv("METRONOME_CONFIG_PATH", tmpfile.Name())
	defer os.Unsetenv("METRONOME_CONFIG_PATH")
	loader, err := NewConfigLoader()
	if err != nil {
		t.Fatalf("NewConfigLoader failed: %v", err)
	}
	defer loader.Stop()
	loader.Start()

	select {
	case config := <-loader.Changes():
		if len(config.Probes) != 1 {
			t.Errorf("Expected 1 probe, got %d", len(config.Probes))
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for initial config")
	}

	err = os.WriteFile(tmpfile.Name(), []byte(`
probes:
  - name: "test_probe"
    proto: "http"
    target: "http://localhost:8080"
  - name: "test_probe_2"
    proto: "http"
    target: "http://localhost:9090"
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case config := <-loader.Changes():
		if len(config.Probes) != 2 {
			t.Errorf("Expected 2 probes after reload, got %d", len(config.Probes))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for config reload")
	}
}

func TestConfigLoader_HashOptimization(t *testing.T) {
	content := []byte(`
probes:
  - name: "test_probe"
    proto: "http"
    target: "http://localhost:8080"
`)
	tmpfile, err := os.CreateTemp("", "config_hash.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	os.Setenv("METRONOME_CONFIG_PATH", tmpfile.Name())
	defer os.Unsetenv("METRONOME_CONFIG_PATH")

	loader, err := NewConfigLoader()
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Stop()

	// Drain the initial config from the channel to avoid blocking in subsequent loadConfig calls
	<-loader.Changes()

	// Initial load
	if len(loader.configHash) == 0 {
		t.Fatal("configHash should be set after initial load")
	}
	initialHash := make([]byte, len(loader.configHash))
	copy(initialHash, loader.configHash)

	// Call loadConfig again with same content
	err = loader.loadConfig()
	if err != nil {
		t.Errorf("loadConfig failed: %v", err)
	}

	// The hash should remain the same and no new change should be sent
	// (Though NewConfigLoader already sent one change, we are not checking the channel here but the hash)
	if string(loader.configHash) != string(initialHash) {
		t.Error("Hash should not have changed")
	}

	// Modify file slightly (whitespace)
	err = os.WriteFile(tmpfile.Name(), append(content, '\n'), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = loader.loadConfig()
	if err != nil {
		t.Errorf("loadConfig failed after whitespace change: %v", err)
	}

	if string(loader.configHash) == string(initialHash) {
		t.Error("Hash should have changed after whitespace modification")
	}
}
