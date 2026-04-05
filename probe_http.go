package main

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func isValidStatusCode(code int, successCodes string) bool {
	if successCodes == "" {
		return code >= 200 && code <= 299
	}

	for _, part := range strings.Split(successCodes, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				continue
			}
			low, errLow := strconv.Atoi(rangeParts[0])
			high, errHigh := strconv.Atoi(rangeParts[1])
			if errLow != nil || errHigh != nil {
				continue
			}
			if code >= low && code <= high {
				return true
			}
		} else {
			singleCode, err := strconv.Atoi(part)
			if err != nil {
				continue
			}
			if code == singleCode {
				return true
			}
		}
	}
	return false
}

func runHTTPProbe(p Probe, collector *MetronomeCollector) {
	labels := prometheus.Labels{
		"name":   p.Name,
		"proto":  "http",
		"target": p.Target,
	}
	for k, v := range p.Labels {
		labels[k] = v
	}

	result := ProbeResult{
		Name:          p.Name,
		Labels:        labels,
		Success:       false,
		FailureReason: FailureReasonNone,
	}

	req, err := http.NewRequest("GET", p.Target, nil)
	if err != nil {
		result.FailureReason = FailureReasonHTTPInvalidRequest
		collector.UpdateResult(result)
		return
	}

	userAgent := os.Getenv("METRONOME_HTTP_USER_AGENT")
	if userAgent == "" {
		userAgent = "Metronome"
	}
	req.Header.Set("User-Agent", userAgent)

	var dnsStart, connectStart, tlsStart, wroteRequestTime time.Time
	var resolvedIP string
	httpLatencies := make(map[string]float64)

	trace := &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(_ httptrace.DNSDoneInfo) {
			httpLatencies["dns"] = time.Since(dnsStart).Seconds()
		},
		ConnectStart: func(_, _ string) {
			connectStart = time.Now()
		},
		ConnectDone: func(_, _ string, _ error) {
			httpLatencies["connect"] = time.Since(connectStart).Seconds()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			resolvedIP = info.Conn.RemoteAddr().String()
		},
		TLSHandshakeStart:    func() { tlsStart = time.Now() },
		TLSHandshakeDone:     func(_ tls.ConnectionState, _ error) { httpLatencies["tls"] = time.Since(tlsStart).Seconds() },
		WroteRequest:         func(_ httptrace.WroteRequestInfo) { wroteRequestTime = time.Now() },
		GotFirstResponseByte: func() { httpLatencies["wait_for_response"] = time.Since(wroteRequestTime).Seconds() },
	}

	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	client := &http.Client{
		Timeout: p.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: p.InsecureSkipVerify,
			},
		},
	}

	startTime := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(startTime).Seconds()

	if err != nil {
		result.Latency = latency
		result.FailureReason = classifyError(err)
		result.ResolvedIP = resolvedIP
		collector.UpdateResult(result)
		return
	}
	defer resp.Body.Close()

	result.Success = true
	result.Status = 1
	result.Latency = latency
	result.HTTPLatencies = httpLatencies
	result.ResolvedIP = resolvedIP
	result.HTTPCode = resp.StatusCode

	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		result.TLSExpiry = float64(cert.NotAfter.Unix())

		if time.Now().After(cert.NotAfter) {
			result.Status = 0
			result.FailureReason = FailureReasonTLSCertificateExpired
		}
	}

	if result.Status != 0 {
		if !isValidStatusCode(resp.StatusCode, p.SuccessCodes) {
			result.Status = 0
			result.FailureReason = FailureReasonHTTPStatusCode
		}
	}

	var body []byte
	var bodyReadErr error
	if result.Status != 0 && (p.Contain != "" || p.NotContain != "") {
		body, bodyReadErr = io.ReadAll(resp.Body)
		if bodyReadErr != nil {
			result.Status = 0
			result.FailureReason = FailureReasonHTTPBodyReadError
		}
	}

	if result.Status != 0 {
		if p.Contain != "" {
			if bodyReadErr == nil && !strings.Contains(string(body), p.Contain) {
				result.Status = 0
				result.FailureReason = FailureReasonHTTPBodyContains
			}
		}

		if p.NotContain != "" {
			if bodyReadErr == nil && strings.Contains(string(body), p.NotContain) {
				result.Status = 0
				result.FailureReason = FailureReasonHTTPBodyNotContains
			}
		}
	}

	if result.Status == 0 {
		result.Success = false
		if result.FailureReason == FailureReasonNone {
			result.FailureReason = FailureReasonHTTPInvalidRequest
		}
	}
	collector.UpdateResult(result)
}
