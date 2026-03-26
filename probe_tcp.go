package main

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func runTCPProbe(ctx context.Context, p Probe, collector *MetronomeCollector) {
	startTime := time.Now()
	labels := prometheus.Labels{
		"name":   p.Name,
		"proto":  "tcp",
		"target": p.Target,
	}
	for k, v := range p.Labels {
		labels[k] = v
	}

	conn, err := net.DialTimeout("tcp", p.Target, p.Timeout)
	latency := time.Since(startTime).Seconds()

	result := ProbeResult{
		Name:    p.Name,
		Labels:  labels,
		Success: false,
	}

	if err != nil {
		result.Latency = latency
		result.Success = false
		result.Status = 0
		result.FailureReason = classifyError(err)
		collector.UpdateResult(result)
		return
	}
	defer conn.Close()

	result.Success = true
	result.Status = 1
	result.Latency = latency

	host, _, err := net.SplitHostPort(p.Target)
	if err != nil {
		host = p.Target
	}

	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: p.InsecureSkipVerify,
	})

	tlsCtx, cancel := context.WithTimeout(ctx, p.Timeout-time.Since(startTime))
	defer cancel()

	if err := tlsConn.HandshakeContext(tlsCtx); err != nil {
		result.Success = false
		result.Status = 0
		result.FailureReason = classifyError(err)
		collector.UpdateResult(result)
		return
	}

	if state := tlsConn.ConnectionState(); len(state.PeerCertificates) > 0 {
		if time.Now().After(state.PeerCertificates[0].NotAfter) {
			result.Success = false
			result.Status = 0
			result.FailureReason = FailureReasonTLSCertificateExpired
		} else {
			result.TLSExpiry = float64(state.PeerCertificates[0].NotAfter.Unix())
		}
	}

	collector.UpdateResult(result)
}
