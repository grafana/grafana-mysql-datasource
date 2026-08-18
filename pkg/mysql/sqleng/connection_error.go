package sqleng

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"strings"
	"syscall"

	"github.com/go-sql-driver/mysql"
)

type HealthErrorCategory string

const (
	HealthErrorCategoryAuth    HealthErrorCategory = "auth"
	HealthErrorCategoryConfig  HealthErrorCategory = "config"
	HealthErrorCategoryNetwork HealthErrorCategory = "network"
	HealthErrorCategoryTimeout HealthErrorCategory = "timeout"
	HealthErrorCategoryTLS     HealthErrorCategory = "tls"
	HealthErrorCategoryServer  HealthErrorCategory = "server"
	HealthErrorCategoryUnknown HealthErrorCategory = "unknown"
)

func classifyHealthError(err error) HealthErrorCategory {
	if err == nil {
		return HealthErrorCategoryUnknown
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return HealthErrorCategoryTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return HealthErrorCategoryTimeout
	}

	if errors.Is(err, mysql.ErrNoTLS) {
		return HealthErrorCategoryTLS
	}
	var certificateVerificationErr *tls.CertificateVerificationError
	var hostnameErr x509.HostnameError
	var unknownAuthorityErr x509.UnknownAuthorityError
	var certificateInvalidErr x509.CertificateInvalidError
	var alertErr tls.AlertError
	var recordHeaderErr tls.RecordHeaderError
	if errors.As(err, &certificateVerificationErr) || errors.As(err, &hostnameErr) ||
		errors.As(err, &unknownAuthorityErr) || errors.As(err, &certificateInvalidErr) ||
		errors.As(err, &alertErr) || errors.As(err, &recordHeaderErr) || hasTLSPrefix(err) {
		return HealthErrorCategoryTLS
	}

	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1044, 1045, 1130, 3118:
			return HealthErrorCategoryAuth
		case 1049, 1298:
			return HealthErrorCategoryConfig
		case 3159:
			return HealthErrorCategoryTLS
		default:
			return HealthErrorCategoryServer
		}
	}

	// Invalid or missing addresses are configuration failures even when the
	// dialer wraps them in net.OpError.
	var addressErr *net.AddrError
	if errors.As(err, &addressErr) {
		return HealthErrorCategoryConfig
	}

	var dnsErr *net.DNSError
	var operationErr *net.OpError
	if errors.As(err, &dnsErr) || errors.As(err, &operationErr) || errors.As(err, &netErr) {
		return HealthErrorCategoryNetwork
	}

	return HealthErrorCategoryUnknown
}

// Some TLS alerts use an unexported standard-library error type. Follow only
// the ordinary unwrap chain and recognize its stable prefix.
func hasTLSPrefix(err error) bool {
	for current := err; current != nil; current = errors.Unwrap(current) {
		if strings.HasPrefix(current.Error(), "tls:") {
			return true
		}
	}
	return false
}

func healthErrorMessage(err error, category HealthErrorCategory) string {
	var message string
	switch category {
	case HealthErrorCategoryAuth:
		message = "[auth] MySQL rejected the configured account. Verify the username, password, and account access."
	case HealthErrorCategoryConfig:
		message = "[config] The MySQL connection settings are invalid. Verify the server address, database, and session settings."
	case HealthErrorCategoryNetwork:
		message = "[network] Grafana could not reach MySQL. Verify the hostname, port, and network access from the Grafana server."
	case HealthErrorCategoryTimeout:
		message = "[timeout] The MySQL connection timed out. Verify server availability, firewall rules, and timeout settings."
	case HealthErrorCategoryTLS:
		message = "[tls] The MySQL TLS connection failed. Verify the datasource TLS settings and server certificate."
	case HealthErrorCategoryServer:
		message = "[server] MySQL rejected the connection. Check server availability, capacity, and server logs."
	default:
		message = "[unknown] The MySQL connection failed. Review the datasource configuration and MySQL server logs."
	}

	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return message + " MySQL refused the connection; verify that it is running and accepting connections on the configured port."
	case errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.EHOSTUNREACH):
		return message + " The configured destination is unreachable from the Grafana server."
	case errors.Is(err, io.EOF):
		return message + " MySQL closed the connection during connection setup."
	}

	var addressErr *net.AddrError
	if errors.As(err, &addressErr) {
		return message + " The server address is malformed; enter it as host:port without an http://, https://, or mysql:// prefix."
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return message + " The configured hostname or service name could not be resolved."
	}

	return message
}
