package main

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTCPProbe_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	p := Probe{
		Name:               "test_tcp_server",
		Proto:              "tcp",
		Target:             ln.Addr().String(),
		Timeout:            1 * time.Second,
		InsecureSkipVerify: true,
	}

	collector := NewMetronomeCollector()
	runTCPProbe(context.Background(), p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.True(t, result.Success)
	assert.Equal(t, float64(0), result.TLSExpiry)
}

func TestTCPProbe_ConnectionRefused(t *testing.T) {
	p := Probe{
		Name:    "test_tcp_refused",
		Proto:   "tcp",
		Target:  "127.0.0.1:1",
		Timeout: 1 * time.Second,
	}

	collector := NewMetronomeCollector()
	runTCPProbe(context.Background(), p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, FailureReasonConnectionRefused, result.FailureReason)
}

func TestTCPProbe_ConnectionTimeout(t *testing.T) {
	p := Probe{
		Name:    "test_timeout",
		Proto:   "tcp",
		Target:  "192.0.2.1:12345", // RFC 5735 non-routable TEST-NET-1
		Timeout: 100 * time.Millisecond,
	}

	collector := NewMetronomeCollector()
	runTCPProbe(context.Background(), p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, FailureReasonConnectionTimeout, result.FailureReason)
}

func TestTCPProbe_DNSFailure(t *testing.T) {
	p := Probe{
		Name:    "test_tcp_dns_failure",
		Proto:   "tcp",
		Target:  "non-existent-domain.invalid:80",
		Timeout: 1 * time.Second,
	}

	collector := NewMetronomeCollector()
	runTCPProbe(context.Background(), p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, FailureReasonDNSResolutionError, result.FailureReason)
}

func TestTCPProbe_TLS_Success(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := Probe{
		Name:               "test_tls_server",
		Proto:              "tcp",
		Target:             server.Listener.Addr().String(),
		Timeout:            1 * time.Second,
		InsecureSkipVerify: true,
		TLS:                true,
	}

	collector := NewMetronomeCollector()
	runTCPProbe(context.Background(), p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.True(t, result.Success)
	assert.Greater(t, result.TLSExpiry, 0.0)
}

func TestTCPProbe_TLS_HandshakeError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Close()
	}()

	p := Probe{
		Name:    "test_handshake_error",
		Proto:   "tcp",
		Target:  ln.Addr().String(),
		Timeout: 1 * time.Second,
		TLS:     true,
	}

	collector := NewMetronomeCollector()
	runTCPProbe(context.Background(), p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, FailureReasonTLSHandshakeError, result.FailureReason)
}

func TestTCPProbe_TLS_HostnameError(t *testing.T) {
	certPEM, keyPEM, err := generateSelfSignedCertWithExpiryAndNames(time.Now().Add(time.Hour), "example.com", []string{"example.com"}, nil)
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		tlsLn := tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
		for {
			conn, err := tlsLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				_ = c.(*tls.Conn).Handshake()
				c.Close()
			}(conn)
		}
	}()

	p := Probe{
		Name:               "test_tcp_hostname_error",
		Proto:              "tcp",
		Target:             ln.Addr().String(),
		Timeout:            1 * time.Second,
		InsecureSkipVerify: false,
		TLS:                true,
	}

	collector := NewMetronomeCollector()
	runTCPProbe(context.Background(), p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, FailureReasonTLSHostnameError, result.FailureReason)
}

func TestTCPProbe_TLS_UnknownAuthority(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := Probe{
		Name:               "test_unknown_authority",
		Proto:              "tcp",
		Target:             server.Listener.Addr().String(),
		Timeout:            1 * time.Second,
		InsecureSkipVerify: false,
		TLS:                true,
	}

	collector := NewMetronomeCollector()
	runTCPProbe(context.Background(), p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, FailureReasonTLSUnknownAuthority, result.FailureReason)
}

func TestTCPProbe_TLS_CertificateExpired(t *testing.T) {
	certPEM, keyPEM, err := generateSelfSignedCertWithExpiry(time.Now().Add(-time.Hour))
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		tlsLn := tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
		for {
			conn, err := tlsLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				_ = c.(*tls.Conn).Handshake()
				c.Close()
			}(conn)
		}
	}()

	p := Probe{
		Name:               "test_tcp_expired_cert",
		Proto:              "tcp",
		Target:             ln.Addr().String(),
		Timeout:            1 * time.Second,
		InsecureSkipVerify: false,
		TLS:                true,
	}

	collector := NewMetronomeCollector()
	runTCPProbe(context.Background(), p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, FailureReasonTLSCertificateExpired, result.FailureReason)
}

func TestTCPProbe_TLS_HandshakeFailed_NoFallback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
		conn.Close()
	}()

	p := Probe{
		Name:               "test_handshake_error_no_fallback",
		Proto:              "tcp",
		Target:             ln.Addr().String(),
		Timeout:            1 * time.Second,
		InsecureSkipVerify: false,
		TLS:                true,
	}

	collector := NewMetronomeCollector()
	runTCPProbe(context.Background(), p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, FailureReasonTLSHandshakeError, result.FailureReason)
}
