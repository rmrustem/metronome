package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"syscall"
	"testing"
	"time"

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

func generateSelfSignedCert() (certPEM, keyPEM []byte, err error) {
	return generateSelfSignedCertWithExpiryAndNames(time.Now().Add(time.Hour), "localhost", []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")})
}

func generateSelfSignedCertWithExpiry(expiry time.Time) (certPEM, keyPEM []byte, err error) {
	return generateSelfSignedCertWithExpiryAndNames(expiry, "localhost", []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")})
}

func generateSelfSignedCertWithExpiryAndNames(expiry time.Time, commonName string, dnsNames []string, ipAddresses []net.IP) (certPEM, keyPEM []byte, err error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-2 * time.Hour),
		NotAfter:              expiry,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
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
