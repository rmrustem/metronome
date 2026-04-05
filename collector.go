package main

import (
	"log/slog"
	"sort"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type ProbeResult struct {
	Name          string
	Labels        prometheus.Labels
	FailureReason int
	Requests      uint64
	Latency       float64
	HTTPLatencies map[string]float64
	HTTPCode      int
	ResolvedIP    string
	TLSExpiry     float64
}

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

		ch <- prometheus.MustNewConstMetric(c.probeRequestsDesc, prometheus.CounterValue, float64(res.Requests), labelValues...)

		ch <- prometheus.MustNewConstMetric(c.probeFailureReasonDesc, prometheus.GaugeValue, float64(res.FailureReason), labelValues...)

		if res.FailureReason == FailureReasonNone {
			ch <- prometheus.MustNewConstMetric(c.probeStatusDesc, prometheus.GaugeValue, 1, labelValues...)
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

func (c *MetronomeCollector) UpdateLabelKeys(keys []string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.labelKeys = make([]string, len(keys))
	copy(c.labelKeys, keys)
	sort.Strings(c.labelKeys)
	c.createDescs()
}

func (c *MetronomeCollector) UpdateResult(res ProbeResult) {
	if res.FailureReason != FailureReasonNone {
		args := make([]any, 0, len(res.Labels)*2+2)
		for k, v := range res.Labels {
			args = append(args, k, v)
		}
		args = append(args, "reason", res.FailureReason)
		if res.ResolvedIP != "" {
			args = append(args, "resolved_ip", res.ResolvedIP)
		}
		if res.HTTPCode != 0 {
			args = append(args, "http_code", res.HTTPCode)
		}
		slog.Warn("Probe failed", args...)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if res.Labels == nil {
		res.Labels = make(prometheus.Labels)
	}

	if _, ok := res.Labels["name"]; !ok {
		res.Labels["name"] = res.Name
	}

	if existing, ok := c.results[res.Name]; ok {
		res.Requests = existing.Requests + 1
	} else {
		res.Requests = 1
	}

	c.results[res.Name] = res
}

func (c *MetronomeCollector) RemoveResult(probeName string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.results, probeName)
}
