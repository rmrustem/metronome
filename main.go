package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var collector *MetronomeCollector

type probeRunner struct {
	probe  Probe
	cancel context.CancelFunc
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	loader, err := NewConfigLoader()
	if err != nil {
		slog.Error("Failed to read configuration", "error", err)
		os.Exit(1)
	}
	defer loader.Stop()
	loader.Start()

	initialConfig := <-loader.Changes()
	slog.Info("Initial configuration loaded")

	collector = NewMetronomeCollector()
	prometheus.MustRegister(collector)

	probes := make(map[string]*probeRunner)

	applyConfig := func(newConfig *Config) {
		slog.Info("Applying new configuration")

		labelKeysMap := map[string]bool{
			"name":   true,
			"proto":  true,
			"target": true,
		}

		newProbes := make(map[string]Probe)
		for _, p := range newConfig.Probes {
			proto := strings.ToLower(p.Proto)
			p.PrecalculatedLabels = prometheus.Labels{
				"name":   p.Name,
				"proto":  proto,
				"target": p.Target,
			}
			for k, v := range p.Labels {
				p.PrecalculatedLabels[k] = v
				labelKeysMap[k] = true
			}
			newProbes[p.Name] = p
		}

		var labelKeys []string
		for k := range labelKeysMap {
			labelKeys = append(labelKeys, k)
		}
		collector.UpdateLabelKeys(labelKeys)

		for name, runner := range probes {
			newProbe, exists := newProbes[name]
			if !exists || !runner.probe.Equal(newProbe) {
				slog.Info("Stopping probe", "name", name)
				runner.cancel()
				delete(probes, name)
				collector.RemoveResult(name)
			}
		}

		for name, p := range newProbes {
			if _, ok := probes[name]; !ok {
				slog.Info("Starting probe", "name", name)
				ctx, cancel := context.WithCancel(context.Background())
				runner := &probeRunner{
					probe:  p,
					cancel: cancel,
				}
				probes[name] = runner
				go runProbe(ctx, p, collector)
			}
		}

	}

	applyConfig(initialConfig)

	listenAddr := getEnvStr("METRONOME_WEB_LISTEN", ":8080")
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		slog.Info("Exporting Prometheus metrics", "listen", listenAddr)
		if err := http.ListenAndServe(listenAddr, nil); err != nil {
			slog.Error("Failed to start HTTP server", "error", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case newConfig := <-loader.Changes():
			applyConfig(newConfig)

		case <-sigCh:
			slog.Info("Shutting down")
			for _, runner := range probes {
				runner.cancel()
			}
			return
		}
	}
}
