package main

import (
	"context"
	"crypto/tls"
	"net"
	"time"
)

func runTCPProbe(ctx context.Context, p Probe, collector *MetronomeCollector) {
	startTime := time.Now()
	conn, err := net.DialTimeout("tcp", p.Target, p.Timeout)
	latency := time.Since(startTime).Seconds()

	result := ProbeResult{
		Name:    p.Name,
		Labels:  p.PrecalculatedLabels,
		Latency: latency,
	}

	if err != nil {
		result.FailureReason = classifyError(err)
		collector.UpdateResult(result)
		return
	}
	defer conn.Close()

	result.ResolvedIP = conn.RemoteAddr().String()

	if !p.TLS {
		collector.UpdateResult(result)
		return
	}

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
		result.FailureReason = classifyError(err)
		collector.UpdateResult(result)
		return
	}

	if state := tlsConn.ConnectionState(); len(state.PeerCertificates) > 0 {
		if time.Now().After(state.PeerCertificates[0].NotAfter) {
			result.FailureReason = FailureReasonTLSCertificateExpired
		} else {
			result.TLSExpiry = float64(state.PeerCertificates[0].NotAfter.Unix())
		}
	}
	collector.UpdateResult(result)
}
