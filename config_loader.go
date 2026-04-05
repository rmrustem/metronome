package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type ConfigLoader struct {
	configPath    string
	configURL     string
	config        *Config
	configChanges chan *Config
	cancel        context.CancelFunc
}

func NewConfigLoader() (*ConfigLoader, error) {
	configPath := getEnvStr("METRONOME_CONFIG_PATH", "config.yaml")
	configURL := getEnvStr("METRONOME_CONFIG_URL", "")

	loader := &ConfigLoader{
		configPath:    configPath,
		configURL:     configURL,
		configChanges: make(chan *Config, 1),
	}

	if err := loader.loadConfig(); err != nil {
		return nil, err
	}

	return loader, nil
}

func (c *ConfigLoader) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	go c.watch(ctx)
}

func (c *ConfigLoader) loadConfig() error {
	var data []byte
	var err error

	if c.configURL != "" {
		data, err = downloadConfig(c.configURL)
		if err != nil {
			return err
		}
	} else {
		data, err = os.ReadFile(c.configPath)
		if err != nil {
			return err
		}
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	if err := config.Validate(); err != nil {
		return err
	}

	if c.config == nil || !configsAreEqual(&config, c.config) {
		c.config = &config
		c.configChanges <- &config
		slog.Info("Configuration loaded")
	}

	return nil
}

func (c *ConfigLoader) watch(ctx context.Context) {
	interval := time.Duration(getEnvInt("METRONOME_CONFIG_RELOAD_INTERVAL", 60)) * time.Second
	if interval == 0 {
		slog.Info("Configuration reloading is disabled")
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.loadConfig(); err != nil {
				slog.Error("Failed to reload config", "error", err)
			}
		}
	}
}

func (c *ConfigLoader) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *ConfigLoader) Changes() <-chan *Config {
	return c.configChanges
}

func downloadConfig(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	authHeader := os.Getenv("METRONOME_CONFIG_URL_AUTH")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download config: status code %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func configsAreEqual(a, b *Config) bool {
	if len(a.Probes) != len(b.Probes) {
		return false
	}
	aProbes := make(map[string]Probe)
	for _, p := range a.Probes {
		aProbes[p.Name] = p
	}
	for _, p := range b.Probes {
		if ap, ok := aProbes[p.Name]; !ok || !ap.Equal(p) {
			return false
		}
	}
	return true
}

func getEnvStr(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}
