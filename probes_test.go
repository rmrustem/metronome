package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{
			name:     "Nil error",
			err:      nil,
			expected: FailureReasonNone,
		},
		{
			name:     "DNS Error",
			err:      &net.DNSError{Err: "no such host", Name: "example.com"},
			expected: FailureReasonDNSResolutionError,
		},
		{
			name:     "Connection Timeout",
			err:      &net.OpError{Op: "dial", Err: &timeoutError{}},
			expected: FailureReasonConnectionTimeout,
		},
		{
			name:     "Connection Refused",
			err:      syscall.ECONNREFUSED,
			expected: FailureReasonConnectionRefused,
		},
		{
			name:     "Unknown Authority",
			err:      x509.UnknownAuthorityError{},
			expected: FailureReasonTLSUnknownAuthority,
		},
		{
			name:     "Hostname Error",
			err:      x509.HostnameError{},
			expected: FailureReasonTLSHostnameError,
		},
		{
			name:     "Certificate Expired",
			err:      x509.CertificateInvalidError{Reason: x509.Expired},
			expected: FailureReasonTLSCertificateExpired,
		},
		{
			name:     "Certificate Invalid Other",
			err:      x509.CertificateInvalidError{Reason: x509.NotAuthorizedToSign},
			expected: FailureReasonTLSCertificateInvalid,
		},
		{
			name:     "TLS Record Header Error",
			err:      tls.RecordHeaderError{Msg: "invalid record header"},
			expected: FailureReasonTLSHandshakeError,
		},
		{
			name:     "Generic Error",
			err:      errors.New("some other error"),
			expected: FailureReasonTLSHandshakeError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, classifyError(tt.err))
		})
	}
}

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }
