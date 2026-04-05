package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPProbe_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := Probe{
		Name:    "test_http_success",
		Proto:   "http",
		Target:  server.URL,
		Timeout: 1 * time.Second,
		Labels:  map[string]string{"service": "test-svc"},
	}

	collector := NewMetronomeCollector()
	runHTTPProbe(p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)

	assert.True(t, result.Success)
	assert.Equal(t, float64(1), result.Status)
	assert.Greater(t, result.Latency, 0.0)
	assert.Equal(t, "test-svc", result.Labels["service"])
	assert.Equal(t, FailureReasonNone, result.FailureReason)
}

func TestHTTPProbe_InvalidURL(t *testing.T) {
	p := Probe{
		Name:    "test_http_invalid_url",
		Proto:   "http",
		Target:  "http://[fe80::1%lo0]:80",
		Timeout: 1 * time.Second,
	}

	collector := NewMetronomeCollector()
	runHTTPProbe(p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, float64(0), result.Status)
	assert.Equal(t, FailureReasonHTTPInvalidRequest, result.FailureReason)
}

func TestHTTPProbe_Non200StatusCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := Probe{
		Name:    "test_http_non_200",
		Proto:   "http",
		Target:  server.URL,
		Timeout: 1 * time.Second,
	}

	collector := NewMetronomeCollector()
	runHTTPProbe(p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, float64(0), result.Status)
	assert.Equal(t, FailureReasonHTTPStatusCode, result.FailureReason)
}

func TestHTTPProbe_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := Probe{
		Name:    "test_http_timeout",
		Proto:   "http",
		Target:  server.URL,
		Timeout: 100 * time.Millisecond,
	}

	collector := NewMetronomeCollector()
	runHTTPProbe(p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, float64(0), result.Status)
	assert.Equal(t, FailureReasonConnectionTimeout, result.FailureReason)
}

func TestHTTPProbe_BodyContain_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, world!"))
	}))
	defer server.Close()

	p := Probe{
		Name:    "test_http_contain_success",
		Proto:   "http",
		Target:  server.URL,
		Timeout: 1 * time.Second,
		Contain: "Hello",
	}
	collector := NewMetronomeCollector()
	runHTTPProbe(p, collector)
	result, _ := collector.results[p.Name]
	assert.True(t, result.Success)
	assert.Equal(t, float64(1), result.Status)
}

func TestHTTPProbe_BodyContain_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, world!"))
	}))
	defer server.Close()

	p := Probe{
		Name:    "test_http_contain_fail",
		Proto:   "http",
		Target:  server.URL,
		Timeout: 1 * time.Second,
		Contain: "Goodbye",
	}
	collector := NewMetronomeCollector()
	runHTTPProbe(p, collector)
	result, _ := collector.results[p.Name]
	assert.False(t, result.Success)
	assert.Equal(t, float64(0), result.Status)
	assert.Equal(t, FailureReasonHTTPBodyContains, result.FailureReason)
}

func TestHTTPProbe_BodyNotContain_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, world!"))
	}))
	defer server.Close()

	p := Probe{
		Name:       "test_http_not_contain_success",
		Proto:      "http",
		Target:     server.URL,
		Timeout:    1 * time.Second,
		NotContain: "Goodbye",
	}
	collector := NewMetronomeCollector()
	runHTTPProbe(p, collector)
	result, _ := collector.results[p.Name]
	assert.True(t, result.Success)
	assert.Equal(t, float64(1), result.Status)
}

func TestHTTPProbe_BodyNotContain_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, world!"))
	}))
	defer server.Close()

	p := Probe{
		Name:       "test_http_not_contain_fail",
		Proto:      "http",
		Target:     server.URL,
		Timeout:    1 * time.Second,
		NotContain: "Hello",
	}
	collector := NewMetronomeCollector()
	runHTTPProbe(p, collector)
	result, _ := collector.results[p.Name]
	assert.False(t, result.Success)
	assert.Equal(t, float64(0), result.Status)
	assert.Equal(t, FailureReasonHTTPBodyNotContains, result.FailureReason)
}

func TestIsValidStatusCode(t *testing.T) {
	testCases := []struct {
		name         string
		code         int
		successCodes string
		expected     bool
	}{
		{"Default success range, valid", 200, "", true},
		{"Default success range, invalid", 300, "", false},
		{"Single success code, valid", 200, "200", true},
		{"Single success code, invalid", 201, "200", false},
		{"Multiple success codes, valid", 201, "200, 201, 202", true},
		{"Multiple success codes, invalid", 203, "200, 201, 202", false},
		{"Range of success codes, valid", 205, "200-299", true},
		{"Range of success codes, invalid", 300, "200-299", false},
		{"Combination of single codes and ranges, valid", 404, "200-299, 404", true},
		{"Combination of single codes and ranges, invalid", 403, "200-299, 404", false},
		{"Edge case, code at start of range", 200, "200-210", true},
		{"Edge case, code at end of range", 210, "200-210", true},
		{"Malformed range", 200, "200-", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := isValidStatusCode(tc.code, tc.successCodes)
			if actual != tc.expected {
				t.Errorf("isValidStatusCode(%d, %q) = %v; want %v", tc.code, tc.successCodes, actual, tc.expected)
			}
		})
	}
}

func TestHTTPProbe_UserAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "Metronome" {
			t.Errorf("Expected User-Agent 'Metronome', got '%s'", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p1 := Probe{
		Name:   "test_http_default_ua",
		Target: server.URL,
	}
	collector1 := NewMetronomeCollector()
	runHTTPProbe(p1, collector1)

	customUA := "MyCustomMetronome/1.0"
	os.Setenv("METRONOME_HTTP_USER_AGENT", customUA)
	defer os.Unsetenv("METRONOME_HTTP_USER_AGENT")

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != customUA {
			t.Errorf("Expected User-Agent '%s', got '%s'", customUA, r.Header.Get("User-Agent"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	p2 := Probe{
		Name:   "test_http_custom_ua",
		Target: server2.URL,
	}
	collector2 := NewMetronomeCollector()
	runHTTPProbe(p2, collector2)
}

func TestHTTPProbe_DNSFailure(t *testing.T) {
	collector := NewMetronomeCollector()
	probe := Probe{
		Name:    "test_http_dns_failure",
		Proto:   "http",
		Target:  "http://non-existent-domain.invalid",
		Timeout: 1 * time.Second,
	}

	runHTTPProbe(probe, collector)

	result, ok := collector.results[probe.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, FailureReasonDNSResolutionError, result.FailureReason)
}

func TestHTTPProbe_ConnectionRefused(t *testing.T) {
	collector := NewMetronomeCollector()
	probe := Probe{
		Name:    "test_http_connection_refused",
		Proto:   "http",
		Target:  "http://127.0.0.1:1",
		Timeout: 1 * time.Second,
	}

	runHTTPProbe(probe, collector)

	result, ok := collector.results[probe.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, FailureReasonConnectionRefused, result.FailureReason)
}

func TestHTTPProbe_TLS_Success(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := Probe{
		Name:               "test_http_tls_success",
		Proto:              "http",
		Target:             server.URL,
		Timeout:            1 * time.Second,
		InsecureSkipVerify: true,
	}

	collector := NewMetronomeCollector()
	runHTTPProbe(p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.True(t, result.Success)
	assert.Greater(t, result.TLSExpiry, 0.0)
}

func TestHTTPProbe_TLS_UnknownAuthority(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := Probe{
		Name:               "test_http_unknown_authority",
		Proto:              "http",
		Target:             server.URL,
		Timeout:            1 * time.Second,
		InsecureSkipVerify: false,
	}

	collector := NewMetronomeCollector()
	runHTTPProbe(p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, FailureReasonTLSUnknownAuthority, result.FailureReason)
}

func TestHTTPProbe_TLS_CertificateExpired(t *testing.T) {
	certPEM, keyPEM, err := generateSelfSignedCertWithExpiry(time.Now().Add(-time.Hour))
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	defer server.Close()

	p := Probe{
		Name:               "test_http_expired_cert",
		Proto:              "http",
		Target:             server.URL,
		Timeout:            1 * time.Second,
		InsecureSkipVerify: false,
	}

	collector := NewMetronomeCollector()
	runHTTPProbe(p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, FailureReasonTLSCertificateExpired, result.FailureReason)
}

func TestHTTPProbe_BodyReadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("too short"))
	}))
	defer server.Close()

	p := Probe{
		Name:    "test_http_body_read_error",
		Proto:   "http",
		Target:  server.URL,
		Timeout: 1 * time.Second,
		Contain: "some-text",
	}

	collector := NewMetronomeCollector()
	runHTTPProbe(p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, float64(0), result.Status)
	assert.Equal(t, FailureReasonHTTPBodyReadError, result.FailureReason)
}

func TestHTTPProbe_TLSHostnameError(t *testing.T) {
	certPEM, keyPEM, err := generateSelfSignedCertWithExpiryAndNames(time.Now().Add(time.Hour), "example.com", []string{"example.com"}, nil)
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	defer server.Close()

	targetURL := strings.Replace(server.URL, "localhost", "127.0.0.1", 1)

	p := Probe{
		Name:               "test_http_hostname_error",
		Proto:              "http",
		Target:             targetURL,
		Timeout:            1 * time.Second,
		InsecureSkipVerify: false,
	}

	collector := NewMetronomeCollector()
	runHTTPProbe(p, collector)

	result, ok := collector.results[p.Name]
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, FailureReasonTLSHostnameError, result.FailureReason)
}
