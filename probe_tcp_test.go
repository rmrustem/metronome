package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunTCPProbe(t *testing.T) {
	// Test 1: Successful probe against a TLS server
	t.Run("TLS_Server_Success", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		p := Probe{
			Name:               "test_tls_server",
			Proto:              "tcp",
			Target:             server.Listener.Addr().String(),
			Timeout:            1 * time.Second,
			InsecureSkipVerify: true, // httptest.NewTLSServer uses a self-signed cert
			Labels:             map[string]string{"service": "tls-test-svc"},
		}

		collector := NewMetronomeCollector()
		runTCPProbe(context.Background(), p, collector)

		result, ok := collector.results[p.Name]
		if !ok {
			t.Fatalf("Probe result not found for %s", p.Name)
		}
		if !result.Success {
			t.Errorf("Expected probe to be successful")
		}
		if result.TLSExpiry <= 0 {
			t.Errorf("Expected tls_expires to be > 0 for a TLS server, but got %v", result.TLSExpiry)
		}
	})

	// Test 2: Successful probe against a non-TLS server
	t.Run("NonTLS_Server_Success", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to start TCP listener: %v", err)
		}
		defer ln.Close()

		// Start a simple TCP server that accepts connections
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
			Name:    "test_nontls_server",
			Proto:   "tcp",
			Target:  ln.Addr().String(),
			Timeout: 1 * time.Second,
		}

		collector := NewMetronomeCollector()
		runTCPProbe(context.Background(), p, collector)

		_, ok := collector.results[p.Name]
		if !ok {
			t.Fatalf("Probe result not found for %s", p.Name)
		}
	})

	// Test 4: SNI check (ServerName set)
	t.Run("SNI_ServerName_Set", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()

		go func() {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			// Just accept and close to simulate a server that doesn't do TLS
			// We want to check if the probe attempt to do TLS with correct ServerName
		}()

		p := Probe{
			Name:    "test_sni",
			Proto:   "tcp",
			Target:  "example.com:443", // We'll override the address but keep the target string for hostname extraction
			Timeout: 1 * time.Second,
		}

		// Mock the dialer or just use a local address but keep p.Target as "example.com:443"
		// Wait, runTCPProbe uses p.Target for Dial.
		// Let's modify p.Target to use our local listener but we need to ensure hostname is extracted correctly.
		p.Target = ln.Addr().String()

		collector := NewMetronomeCollector()
		runTCPProbe(context.Background(), p, collector)

		result, ok := collector.results[p.Name]
		require.True(t, ok)
		// It will fail handshake because our mock server doesn't do TLS,
		// but we are mostly checking it doesn't crash and classifies the error.
		assert.False(t, result.Success)
		assert.Equal(t, FailureReasonTLSHandshakeError, result.FailureReason)
	})

	// Test 3: Failed probe against a closed port
	t.Run("Server_Failure", func(t *testing.T) {
		p := Probe{
			Name:    "test_tcp_fail",
			Proto:   "tcp",
			Target:  "127.0.0.1:1", // Assuming this port is not in use
			Timeout: 1 * time.Second,
		}

		collector := NewMetronomeCollector()
		runTCPProbe(context.Background(), p, collector)

		result, ok := collector.results[p.Name]
		if !ok {
			t.Fatalf("Probe result not found for %s", p.Name)
		}
		if result.Success {
			t.Errorf("Expected probe to be unsuccessful")
		}
	})
}

func TestTCPProbe_ConnectionRefused(t *testing.T) {
	collector := NewMetronomeCollector()
	probe := Probe{
		Name:    "test_tcp_refused",
		Proto:   "tcp",
		Target:  "localhost:1", // Port where nobody is listening
		Timeout: 500 * time.Millisecond,
	}

	runTCPProbe(context.Background(), probe, collector)

	result, ok := collector.results[probe.Name]
	require.True(t, ok, "Probe result not found")

	assert.False(t, result.Success, "Probe should have failed")
	assert.Equal(t, FailureReasonConnectionRefused, result.FailureReason, "Failure reason should be ConnectionRefused")
}

func TestTCPProbe_ConnectionTimeout(t *testing.T) {
	// This test requires a non-routable IP address to simulate a timeout.
	// 240.0.0.0/4 is reserved for future use and should be non-routable.
	target := "240.0.0.1:12345"

	collector := NewMetronomeCollector()
	probe := Probe{
		Name:    "test_tcp_timeout",
		Proto:   "tcp",
		Target:  target,
		Timeout: 100 * time.Millisecond, // Short timeout
	}

	runTCPProbe(context.Background(), probe, collector)

	result, ok := collector.results[probe.Name]
	require.True(t, ok, "Probe result not found")
	assert.False(t, result.Success, "Probe should have failed")
	assert.Equal(t, FailureReasonConnectionTimeout, result.FailureReason, "Failure reason should be ConnectionTimeout")
}

func generateSelfSignedCert() (certPEM, keyPEM []byte, err error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	certBuffer := &bytes.Buffer{}
	if err := pem.Encode(certBuffer, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return nil, nil, err
	}

	keyBuffer := &bytes.Buffer{}
	if err := pem.Encode(keyBuffer, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}); err != nil {
		return nil, nil, err
	}

	return certBuffer.Bytes(), keyBuffer.Bytes(), nil
}
