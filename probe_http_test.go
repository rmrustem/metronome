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

func TestRunHTTPProbe(t *testing.T) {
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
	if !ok {
		t.Fatalf("Probe result not found for %s", p.Name)
	}

	if !result.Success {
		t.Errorf("Expected probe to be successful")
	}

	if result.Status != 1 {
		t.Errorf("Expected status to be 1, but got %v", result.Status)
	}

	if result.Latency <= 0 {
		t.Errorf("Expected latency to be greater than 0, but got %v", result.Latency)
	}

	if result.Labels["service"] != "test-svc" {
		t.Errorf("Expected service label to be 'test-svc', but got %v", result.Labels["service"])
	}
}

func TestRunHTTPProbe_ErrorHandling(t *testing.T) {
	// Test case 1: Invalid URL
	p1 := Probe{
		Name:    "test_http_invalid_url",
		Proto:   "http",
		Target:  "invalid-url",
		Timeout: 1 * time.Second,
	}

	collector1 := NewMetronomeCollector()
	runHTTPProbe(p1, collector1)

	result1, ok := collector1.results[p1.Name]
	if !ok {
		t.Fatalf("Probe result not found for %s", p1.Name)
	}
	if result1.Success {
		t.Errorf("Expected probe to be unsuccessful for invalid URL")
	}

	// Test case 2: Non-200 status code
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // 500 Internal Server Error
	}))
	defer server2.Close()

	p2 := Probe{
		Name:    "test_http_non_200",
		Proto:   "http",
		Target:  server2.URL,
		Timeout: 1 * time.Second,
	}

	collector2 := NewMetronomeCollector()
	runHTTPProbe(p2, collector2)

	result2, ok := collector2.results[p2.Name]
	if !ok {
		t.Fatalf("Probe result not found for %s", p2.Name)
	}
	if result2.Success {
		t.Errorf("Expected probe to be unsuccessful for non-200 status code")
	}

	// Test case 3: Timeout
	server3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Simulate a slow response
		w.WriteHeader(http.StatusOK)
	}))
	defer server3.Close()

	p3 := Probe{
		Name:    "test_http_timeout",
		Proto:   "http",
		Target:  server3.URL,
		Timeout: 1 * time.Second,
	}

	collector3 := NewMetronomeCollector()
	runHTTPProbe(p3, collector3)

	result3, ok := collector3.results[p3.Name]
	if !ok {
		t.Fatalf("Probe result not found for %s", p3.Name)
	}
	if result3.Success {
		t.Errorf("Expected probe to be unsuccessful for timeout")
	}
}

func TestRunHTTPProbe_BodyChecks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, world!"))
	}))
	defer server.Close()

	// Test for successful contain
	pContainSuccess := Probe{
		Name:    "test_http_contain_success",
		Proto:   "http",
		Target:  server.URL,
		Timeout: 1 * time.Second,
		Contain: "Hello, world!",
	}
	collectorContainSuccess := NewMetronomeCollector()
	runHTTPProbe(pContainSuccess, collectorContainSuccess)
	resultContainSuccess, _ := collectorContainSuccess.results[pContainSuccess.Name]
	if !resultContainSuccess.Success {
		t.Errorf("Expected probe_status to be 1 for successful contain check")
	}

	// Test for failed contain
	pContainFail := Probe{
		Name:    "test_http_contain_fail",
		Proto:   "http",
		Target:  server.URL,
		Timeout: 1 * time.Second,
		Contain: "Goodbye, world!",
	}
	collectorContainFail := NewMetronomeCollector()
	runHTTPProbe(pContainFail, collectorContainFail)
	resultContainFail, _ := collectorContainFail.results[pContainFail.Name]
	if resultContainFail.Success {
		t.Errorf("Expected probe_status to be 0 for failed contain check")
	}

	// Test for successful not contain
	pNotContainSuccess := Probe{
		Name:       "test_http_not_contain_success",
		Proto:      "http",
		Target:     server.URL,
		Timeout:    1 * time.Second,
		NotContain: "Goodbye, world!",
	}
	collectorNotContainSuccess := NewMetronomeCollector()
	runHTTPProbe(pNotContainSuccess, collectorNotContainSuccess)
	resultNotContainSuccess, _ := collectorNotContainSuccess.results[pNotContainSuccess.Name]
	if !resultNotContainSuccess.Success {
		t.Errorf("Expected probe_status to be 1 for successful not_contain check")
	}

	// Test for failed not contain
	pNotContainFail := Probe{
		Name:       "test_http_not_contain_fail",
		Proto:      "http",
		Target:     server.URL,
		Timeout:    1 * time.Second,
		NotContain: "Hello, world!",
	}
	collectorNotContainFail := NewMetronomeCollector()
	runHTTPProbe(pNotContainFail, collectorNotContainFail)
	resultNotContainFail, _ := collectorNotContainFail.results[pNotContainFail.Name]
	if resultNotContainFail.Success {
		t.Errorf("Expected probe_status to be 0 for failed not_contain check")
	}
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

func TestRunHTTPProbe_UserAgent(t *testing.T) {
	// Test with default User-Agent
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

	// Test with custom User-Agent from env var
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
		Target:  "http://non-existent-domain.local",
		Timeout: 1 * time.Second,
	}

	runHTTPProbe(probe, collector)

	result, ok := collector.results[probe.Name]
	require.True(t, ok, "Probe result not found")
	assert.False(t, result.Success, "Probe should have failed")
	// Note: Sometimes DNS resolution failure can be reported as a connection timeout or other error depending on the environment
	assert.Contains(t, []int{FailureReasonDNSResolutionError, FailureReasonConnectionTimeout}, result.FailureReason, "Failure reason should be DNSResolutionError or ConnectionTimeout")
}

func TestHTTPProbe_ConnectionTimeout(t *testing.T) {
	// This test requires a non-routable IP address to simulate a timeout.
	// 192.0.2.1 is from TEST-NET-1, reserved for documentation and should be non-routable.
	target := "http://192.0.2.1"

	collector := NewMetronomeCollector()
	probe := Probe{
		Name:    "test_http_timeout",
		Proto:   "http",
		Target:  target,
		Timeout: 200 * time.Millisecond, // Short timeout to make the test faster
	}

	runHTTPProbe(probe, collector)

	result, ok := collector.results[probe.Name]
	require.True(t, ok, "Probe result not found")
	assert.False(t, result.Success, "Probe should have failed")
	assert.Equal(t, FailureReasonConnectionTimeout, result.FailureReason, "Failure reason should be ConnectionTimeout")
}

func TestHTTPProbe_StatusCodeFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	collector := NewMetronomeCollector()
	probe := Probe{
		Name:         "test_http_status_code",
		Proto:        "http",
		Target:       server.URL,
		SuccessCodes: "200-299", // Default success codes
		Timeout:      1 * time.Second,
	}

	runHTTPProbe(probe, collector)

	result, ok := collector.results[probe.Name]
	require.True(t, ok, "Probe result not found")
	assert.False(t, result.Success, "Probe should have failed")
	assert.Equal(t, FailureReasonHTTPStatusCode, result.FailureReason, "Failure reason should be HTTPStatusCode")
}

func TestHTTPProbe_BodyContainsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, world!"))
	}))
	defer server.Close()

	collector := NewMetronomeCollector()
	probe := Probe{
		Name:    "test_http_body_contains",
		Proto:   "http",
		Target:  server.URL,
		Contain: "should not be found",
		Timeout: 1 * time.Second,
	}

	runHTTPProbe(probe, collector)

	result, ok := collector.results[probe.Name]
	require.True(t, ok, "Probe result not found")
	assert.False(t, result.Success, "Probe should have failed")
	assert.Equal(t, FailureReasonHTTPBodyContains, result.FailureReason, "Failure reason should be HTTPBodyContains")
}

func TestHTTPProbe_BodyNotContainsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("This text should be found"))
	}))
	defer server.Close()

	collector := NewMetronomeCollector()
	probe := Probe{
		Name:       "test_http_body_not_contains",
		Proto:      "http",
		Target:     server.URL,
		NotContain: "should be found",
		Timeout:    1 * time.Second,
	}

	runHTTPProbe(probe, collector)

	result, ok := collector.results[probe.Name]
	require.True(t, ok, "Probe result not found")
	assert.False(t, result.Success, "Probe should have failed")
	assert.Equal(t, FailureReasonHTTPBodyNotContains, result.FailureReason, "Failure reason should be HTTPBodyNotContains")
}

func TestTLSCertificateInvalid(t *testing.T) {
	// Create a self-signed certificate for the server
	serverCert, serverKey, err := generateSelfSignedCert()
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(serverCert, serverKey)
	require.NoError(t, err)

	// Start a TLS server with the self-signed certificate
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	defer server.Close()

	// The probe will fail because the client doesn't trust the self-signed cert
	collector := NewMetronomeCollector()
	probe := Probe{
		Name:    "test_tls_invalid_cert",
		Proto:   "http",
		Target:  strings.Replace(server.URL, "127.0.0.1", "localhost", 1), // Use localhost for cert validation
		Timeout: 1 * time.Second,
		// Set InsecureSkipVerify to false to enable TLS verification
		InsecureSkipVerify: false,
	}

	runHTTPProbe(probe, collector)
	result, found := collector.results[probe.Name]
	require.True(t, found)

	assert.False(t, result.Success, "Probe should have failed due to invalid TLS certificate")
	assert.Contains(t, []int{FailureReasonTLSUnknownAuthority, FailureReasonTLSHandshakeError}, result.FailureReason, "Expected FailureReasonTLSUnknownAuthority or FailureReasonTLSHandshakeError")
}
