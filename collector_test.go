package main

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetronomeCollector_DynamicLabels(t *testing.T) {
	c := NewMetronomeCollector()
	c.UpdateLabelKeys([]string{"name", "proto", "target", "region", "service"})
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	c.UpdateResult(ProbeResult{
		Name:    "probe1",
		Labels:  prometheus.Labels{"name": "probe1", "proto": "tcp", "target": "a"},
		Latency: 0.1,
	})

	c.UpdateResult(ProbeResult{
		Name:      "probe2",
		Labels:    prometheus.Labels{"name": "probe2", "proto": "https", "target": "b", "region": "us-east-1"},
		Latency:   0.2,
		TLSExpiry: 1234567890,
	})

	c.UpdateResult(ProbeResult{
		Name:    "probe3",
		Labels:  prometheus.Labels{"name": "probe3", "proto": "http", "target": "c", "service": "web"},
		Latency: 0.3,
	})

	c.UpdateResult(ProbeResult{
		Name:    "probe1",
		Labels:  prometheus.Labels{"name": "probe1", "proto": "tcp", "target": "a"},
		Latency: 0.11,
	})

	expected := `
		# HELP metronome_probe_failure_reason Reason code for probe failure (0 for success)
		# TYPE metronome_probe_failure_reason gauge
		metronome_probe_failure_reason{name="probe1",proto="tcp",region="",service="",target="a"} 0
		metronome_probe_failure_reason{name="probe2",proto="https",region="us-east-1",service="",target="b"} 0
		metronome_probe_failure_reason{name="probe3",proto="http",region="",service="web",target="c"} 0
		# HELP metronome_probe_latency_seconds Total round-trip time in seconds
		# TYPE metronome_probe_latency_seconds gauge
		metronome_probe_latency_seconds{name="probe1",proto="tcp",region="",service="",target="a"} 0.11
		metronome_probe_latency_seconds{name="probe2",proto="https",region="us-east-1",service="",target="b"} 0.2
		metronome_probe_latency_seconds{name="probe3",proto="http",region="",service="web",target="c"} 0.3
		# HELP metronome_probe_requests_total Total number of requests for each probe
		# TYPE metronome_probe_requests_total counter
		metronome_probe_requests_total{name="probe1",proto="tcp",region="",service="",target="a"} 2
		metronome_probe_requests_total{name="probe2",proto="https",region="us-east-1",service="",target="b"} 1
		metronome_probe_requests_total{name="probe3",proto="http",region="",service="web",target="c"} 1
		# HELP metronome_probe_status Probe status (1 for success, 0 for failure)
		# TYPE metronome_probe_status gauge
		metronome_probe_status{name="probe1",proto="tcp",region="",service="",target="a"} 1
		metronome_probe_status{name="probe2",proto="https",region="us-east-1",service="",target="b"} 1
		metronome_probe_status{name="probe3",proto="http",region="",service="web",target="c"} 1
		# HELP metronome_tls_expires Unix timestamp of the server certificate expiration
		# TYPE metronome_tls_expires gauge
		metronome_tls_expires{name="probe2",proto="https",region="us-east-1",service="",target="b"} 1.23456789e+09
	`

	if err := testutil.CollectAndCompare(c, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected collecting result:\n%s", err)
	}

	c.RemoveResult("probe2")

	expectedAfterRemove := `
		# HELP metronome_probe_failure_reason Reason code for probe failure (0 for success)
		# TYPE metronome_probe_failure_reason gauge
		metronome_probe_failure_reason{name="probe1",proto="tcp",region="",service="",target="a"} 0
		metronome_probe_failure_reason{name="probe3",proto="http",region="",service="web",target="c"} 0
		# HELP metronome_probe_latency_seconds Total round-trip time in seconds
		# TYPE metronome_probe_latency_seconds gauge
		metronome_probe_latency_seconds{name="probe1",proto="tcp",region="",service="",target="a"} 0.11
		metronome_probe_latency_seconds{name="probe3",proto="http",region="",service="web",target="c"} 0.3
		# HELP metronome_probe_requests_total Total number of requests for each probe
		# TYPE metronome_probe_requests_total counter
		metronome_probe_requests_total{name="probe1",proto="tcp",region="",service="",target="a"} 2
		metronome_probe_requests_total{name="probe3",proto="http",region="",service="web",target="c"} 1
		# HELP metronome_probe_status Probe status (1 for success, 0 for failure)
		# TYPE metronome_probe_status gauge
		metronome_probe_status{name="probe1",proto="tcp",region="",service="",target="a"} 1
		metronome_probe_status{name="probe3",proto="http",region="",service="web",target="c"} 1
	`
	if err := testutil.CollectAndCompare(c, strings.NewReader(expectedAfterRemove)); err != nil {
		t.Errorf("unexpected collecting result after remove:\n%s", err)
	}
}
