package sqleng

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

type timedOutError struct{}

func (timedOutError) Error() string   { return "raw timeout error" }
func (timedOutError) Timeout() bool   { return true }
func (timedOutError) Temporary() bool { return true }

func TestClassifyHealthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want HealthErrorCategory
	}{
		{name: "MySQL 1044", err: &mysql.MySQLError{Number: 1044}, want: HealthErrorCategoryAuth},
		{name: "MySQL 1045", err: &mysql.MySQLError{Number: 1045}, want: HealthErrorCategoryAuth},
		{name: "MySQL 1130", err: &mysql.MySQLError{Number: 1130}, want: HealthErrorCategoryAuth},
		{name: "MySQL 1049", err: &mysql.MySQLError{Number: 1049}, want: HealthErrorCategoryConfig},
		{name: "MySQL 1298", err: &mysql.MySQLError{Number: 1298}, want: HealthErrorCategoryConfig},
		{name: "MySQL 3159", err: &mysql.MySQLError{Number: 3159}, want: HealthErrorCategoryTLS},
		{name: "other MySQL error", err: &mysql.MySQLError{Number: 1040}, want: HealthErrorCategoryServer},
		{name: "wrapped MySQL error", err: fmt.Errorf("connect: %w", &mysql.MySQLError{Number: 1045}), want: HealthErrorCategoryAuth},
		{name: "invalid address", err: &net.OpError{Err: &net.AddrError{Err: "missing port", Addr: "invalid-address"}}, want: HealthErrorCategoryConfig},
		{name: "DNS failure", err: &net.DNSError{Err: "no such host", Name: "invalid.example"}, want: HealthErrorCategoryNetwork},
		{name: "connection refused", err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, want: HealthErrorCategoryNetwork},
		{name: "context deadline", err: context.DeadlineExceeded, want: HealthErrorCategoryTimeout},
		{name: "timed out network error", err: &net.OpError{Err: timedOutError{}}, want: HealthErrorCategoryTimeout},
		{name: "server without TLS", err: mysql.ErrNoTLS, want: HealthErrorCategoryTLS},
		{name: "server without TLS wrapped by network operation", err: &net.OpError{Err: mysql.ErrNoTLS}, want: HealthErrorCategoryTLS},
		{name: "certificate validation", err: x509.UnknownAuthorityError{}, want: HealthErrorCategoryTLS},
		{name: "TLS alert", err: tls.AlertError(40), want: HealthErrorCategoryTLS},
		{name: "TLS record header", err: tls.RecordHeaderError{}, want: HealthErrorCategoryTLS},
		{name: "TLS alert wrapped by network operation", err: &net.OpError{Err: errors.New("tls: handshake failure")}, want: HealthErrorCategoryTLS},
		{name: "unknown", err: errors.New("unrecognized error"), want: HealthErrorCategoryUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, classifyHealthError(tt.err))
		})
	}
}

func TestHealthErrorMessagesIncludeCategory(t *testing.T) {
	for _, category := range []HealthErrorCategory{
		HealthErrorCategoryAuth,
		HealthErrorCategoryConfig,
		HealthErrorCategoryNetwork,
		HealthErrorCategoryTimeout,
		HealthErrorCategoryTLS,
		HealthErrorCategoryServer,
		HealthErrorCategoryUnknown,
	} {
		message := healthErrorMessage(errors.New("raw upstream error"), category)
		require.Contains(t, message, "["+string(category)+"]")
		require.Greater(t, len(message), len(category)+2)
	}
}

func TestHealthErrorMessageAddsFixedNetworkGuidance(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		category HealthErrorCategory
		want     string
	}{
		{
			name:     "connection refused",
			err:      &net.OpError{Op: "dial", Net: "tcp", Err: fmt.Errorf("invalid-host:3306: %w", syscall.ECONNREFUSED)},
			category: HealthErrorCategoryNetwork,
			want:     "[network] Grafana could not reach MySQL. Verify the hostname, port, and network access from the Grafana server. MySQL refused the connection; verify that it is running and accepting connections on the configured port.",
		},
		{
			name:     "destination unreachable",
			err:      &net.OpError{Op: "dial", Net: "tcp", Err: fmt.Errorf("invalid-host:3306: %w", syscall.EHOSTUNREACH)},
			category: HealthErrorCategoryNetwork,
			want:     "[network] Grafana could not reach MySQL. Verify the hostname, port, and network access from the Grafana server. The configured destination is unreachable from the Grafana server.",
		},
		{
			name:     "connection closed",
			err:      &net.OpError{Op: "read", Net: "tcp", Err: fmt.Errorf("invalid-host:3306: %w", io.EOF)},
			category: HealthErrorCategoryNetwork,
			want:     "[network] Grafana could not reach MySQL. Verify the hostname, port, and network access from the Grafana server. MySQL closed the connection during connection setup.",
		},
		{
			name:     "malformed address",
			err:      &net.OpError{Op: "dial", Net: "tcp", Err: &net.AddrError{Err: "too many colons in address", Addr: "invalid-host"}},
			category: HealthErrorCategoryConfig,
			want:     "[config] The MySQL connection settings are invalid. Verify the server address, database, and session settings. The server address is malformed; enter it as host:port without an http://, https://, or mysql:// prefix.",
		},
		{
			name:     "DNS failure",
			err:      &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "invalid-host"}},
			category: HealthErrorCategoryNetwork,
			want:     "[network] Grafana could not reach MySQL. Verify the hostname, port, and network access from the Grafana server. The configured hostname or service name could not be resolved.",
		},
		{
			name:     "unrecognized network failure",
			err:      &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("raw upstream error")},
			category: HealthErrorCategoryNetwork,
			want:     "[network] Grafana could not reach MySQL. Verify the hostname, port, and network access from the Grafana server.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := healthErrorMessage(tt.err, tt.category)
			require.Equal(t, tt.want, message)
			require.NotContains(t, message, "invalid-host")
			require.NotContains(t, message, "raw upstream error")
		})
	}
}
