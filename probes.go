package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	FailureReasonNone                  = 0
	FailureReasonDNSResolutionError    = 1001
	FailureReasonConnectionTimeout     = 1101
	FailureReasonConnectionRefused     = 1102
	FailureReasonTLSHandshakeError     = 1201
	FailureReasonTLSCertificateExpired = 1202
	FailureReasonTLSUnknownAuthority   = 1203
	FailureReasonTLSHostnameError      = 1204
	FailureReasonTLSCertificateInvalid = 1205
	FailureReasonHTTPInvalidRequest    = 1300
	FailureReasonHTTPStatusCode        = 1301
	FailureReasonHTTPBodyReadError     = 1302
	FailureReasonHTTPBodyContains      = 1303
	FailureReasonHTTPBodyNotContains   = 1304
)

var probeInterval = time.Duration(getEnvInt("METRONOME_PROBE_INTERVAL", 30)) * time.Second

func newRand() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

func runProbe(ctx context.Context, p Probe, collector *MetronomeCollector) {
	if probeInterval >= 2*time.Second {
		initialDelay := time.Duration(newRand().Int63n(int64(probeInterval)))
		select {
		case <-time.After(initialDelay):
		case <-ctx.Done():
			return
		}
	}

	ticker := time.NewTicker(probeInterval)
	defer ticker.Stop()

	for {
		switch strings.ToLower(p.Proto) {
		case "tcp":
			go runTCPProbe(ctx, p, collector)
		case "http":
			go runHTTPProbe(p, collector)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return fallback
}

func classifyError(err error) int {
	if err == nil {
		return FailureReasonNone
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return FailureReasonDNSResolutionError
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return FailureReasonConnectionTimeout
	}

	if errors.Is(err, syscall.ECONNREFUSED) {
		return FailureReasonConnectionRefused
	}

	var unknownAuthErr x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthErr) {
		return FailureReasonTLSUnknownAuthority
	}
	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return FailureReasonTLSHostnameError
	}

	var x509CertInvalidErr x509.CertificateInvalidError
	if errors.As(err, &x509CertInvalidErr) {
		if x509CertInvalidErr.Reason == x509.Expired {
			return FailureReasonTLSCertificateExpired
		}
		return FailureReasonTLSCertificateInvalid
	}

	if _, ok := err.(tls.RecordHeaderError); ok {
		return FailureReasonTLSHandshakeError
	}

	return FailureReasonTLSHandshakeError
}
