package main

import (
	"sort"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// ProbeResult holds the outcome of a single probe execution.
type ProbeResult struct {
	Name          string
	Labels        prometheus.Labels
	Status        float64
	Latency       float64
	TLSExpiry     float64
	HTTPLatencies map[string]float64
	Success       bool
	FailureReason int
	Requests      float64
}

// MetronomeCollector implements the prometheus.Collector interface.
type MetronomeCollector struct {
	probeStatusDesc        *prometheus.Desc
	probeLatencyDesc       *prometheus.Desc
	tlsExpiresDesc         *prometheus.Desc
	httpLatencyDesc        *prometheus.Desc
	probeRequestsDesc      *prometheus.Desc
	probeFailureReasonDesc *prometheus.Desc

	labelKeys []string
	results   map[string]ProbeResult
	mutex     sync.RWMutex
}

// NewMetronomeCollector creates a new, empty collector.
func NewMetronomeCollector() *MetronomeCollector {
	return &MetronomeCollector{
		labelKeys: []string{"name", "proto", "target"},
		results:   make(map[string]ProbeResult),
	}
}

func (c *MetronomeCollector) createDescs() {
	c.probeStatusDesc = prometheus.NewDesc("metronome_probe_status", "Probe status (1 for success, 0 for failure)", c.labelKeys, nil)
	c.probeLatencyDesc = prometheus.NewDesc("metronome_probe_latency_seconds", "Total round-trip time in seconds", c.labelKeys, nil)
	c.tlsExpiresDesc = prometheus.NewDesc("metronome_tls_expires", "Unix timestamp of the server certificate expiration", c.labelKeys, nil)
	c.httpLatencyDesc = prometheus.NewDesc("metronome_http_latency_seconds", "HTTP latency by phase", []string{"name", "proto", "target", "phase"}, nil)
	c.probeRequestsDesc = prometheus.NewDesc("metronome_probe_requests_total", "Total number of requests for each probe", c.labelKeys, nil)
	c.probeFailureReasonDesc = prometheus.NewDesc("metronome_probe_failure_reason", "Reason code for probe failure (0 for success)", c.labelKeys, nil)
}

// Describe implements prometheus.Collector.
func (c *MetronomeCollector) Describe(ch chan<- *prometheus.Desc) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.probeStatusDesc == nil {
		c.createDescs()
	}

	ch <- c.probeStatusDesc
	ch <- c.probeLatencyDesc
	ch <- c.tlsExpiresDesc
	ch <- c.httpLatencyDesc
	ch <- c.probeRequestsDesc
	ch <- c.probeFailureReasonDesc
}

// Collect implements prometheus.Collector.
func (c *MetronomeCollector) Collect(ch chan<- prometheus.Metric) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.probeStatusDesc == nil {
		c.createDescs()
	}

	for _, res := range c.results {
		labelValues := make([]string, len(c.labelKeys))
		for i, key := range c.labelKeys {
			if val, ok := res.Labels[key]; ok {
				labelValues[i] = val
			} else {
				labelValues[i] = ""
			}
		}

		ch <- prometheus.MustNewConstMetric(c.probeRequestsDesc, prometheus.CounterValue, res.Requests, labelValues...)

		// Always emit failure reason, 0 for success
		ch <- prometheus.MustNewConstMetric(c.probeFailureReasonDesc, prometheus.GaugeValue, float64(res.FailureReason), labelValues...)

		if res.Success {
			ch <- prometheus.MustNewConstMetric(c.probeStatusDesc, prometheus.GaugeValue, res.Status, labelValues...)
			ch <- prometheus.MustNewConstMetric(c.probeLatencyDesc, prometheus.GaugeValue, res.Latency, labelValues...)
		} else {
			ch <- prometheus.MustNewConstMetric(c.probeStatusDesc, prometheus.GaugeValue, 0, labelValues...)
		}

		if res.TLSExpiry > 0 {
			ch <- prometheus.MustNewConstMetric(c.tlsExpiresDesc, prometheus.GaugeValue, res.TLSExpiry, labelValues...)
		}

		if res.HTTPLatencies != nil {
			for phase, latency := range res.HTTPLatencies {
				name, _ := res.Labels["name"]
				proto, _ := res.Labels["proto"]
				target, _ := res.Labels["target"]
				httpLabelValues := []string{name, proto, target, phase}
				ch <- prometheus.MustNewConstMetric(c.httpLatencyDesc, prometheus.GaugeValue, latency, httpLabelValues...)
			}
		}
	}
}

func (c *MetronomeCollector) UpdateResult(res ProbeResult) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if _, ok := res.Labels["name"]; !ok {
		res.Labels["name"] = res.Name
	}

	needsDescUpdate := c.probeStatusDesc == nil
	for key := range res.Labels {
		found := false
		for _, existingKey := range c.labelKeys {
			if key == existingKey {
				found = true
				break
			}
		}
		if !found {
			c.labelKeys = append(c.labelKeys, key)
			needsDescUpdate = true
		}
	}

	if needsDescUpdate {
		sort.Strings(c.labelKeys)
		c.createDescs()
	}

	if existing, ok := c.results[res.Name]; ok {
		res.Requests = existing.Requests + 1
	} else {
		res.Requests = 1
	}

	c.results[res.Name] = res
}

// RemoveResult removes a probe result from the collector.
func (c *MetronomeCollector) RemoveResult(probeName string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.results, probeName)
}
