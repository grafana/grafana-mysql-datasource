package sqleng

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"

	"github.com/VividCortex/mysqlerr"
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
	var mysqlErr *mysql.MySQLError
	hasMySQLError := errors.As(err, &mysqlErr)
	if hasMySQLError && mysqlErr == nil {
		return HealthErrorCategoryUnknown
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return HealthErrorCategoryTimeout
	}
	if errors.Is(err, context.Canceled) {
		return HealthErrorCategoryUnknown
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

	if errors.Is(err, mysql.ErrCleartextPassword) || errors.Is(err, mysql.ErrNativePassword) ||
		errors.Is(err, mysql.ErrOldPassword) || errors.Is(err, mysql.ErrUnknownPlugin) {
		return HealthErrorCategoryAuth
	}
	if errors.Is(err, mysql.ErrInvalidConn) || errors.Is(err, mysql.ErrMalformPkt) ||
		errors.Is(err, mysql.ErrPktSync) || errors.Is(err, mysql.ErrPktSyncMul) {
		return HealthErrorCategoryNetwork
	}
	if errors.Is(err, mysql.ErrOldProtocol) {
		return HealthErrorCategoryServer
	}

	if hasMySQLError {
		switch mysqlErr.Number {
		case mysqlerr.ER_DBACCESS_DENIED_ERROR,
			mysqlerr.ER_ACCESS_DENIED_ERROR,
			mysqlerr.ER_HOST_NOT_PRIVILEGED,
			mysqlerr.ER_ACCOUNT_HAS_BEEN_LOCKED,
			mysqlerr.ER_NOT_SUPPORTED_AUTH_MODE,
			mysqlerr.ER_ACCESS_DENIED_NO_PASSWORD_ERROR,
			mysqlerr.ER_MUST_CHANGE_PASSWORD,
			mysqlerr.ER_MUST_CHANGE_PASSWORD_LOGIN:
			return HealthErrorCategoryAuth
		case mysqlerr.ER_BAD_DB_ERROR, mysqlerr.ER_UNKNOWN_TIME_ZONE:
			return HealthErrorCategoryConfig
		case mysqlerr.ER_SECURE_TRANSPORT_REQUIRED:
			return HealthErrorCategoryTLS
		default:
			return HealthErrorCategoryServer
		}
	}

	if errors.Is(err, io.EOF) {
		return HealthErrorCategoryNetwork
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
		message = "MySQL rejected the configured account. Verify the username, password, and account access."
	case HealthErrorCategoryConfig:
		message = "The MySQL connection settings are invalid. Verify the server address, database, and session settings."
	case HealthErrorCategoryNetwork:
		message = "Grafana could not establish a connection to MySQL. Verify the hostname, port, and network access from the Grafana server."
	case HealthErrorCategoryTimeout:
		message = "The MySQL connection timed out. Verify server availability, firewall rules, and timeout settings."
	case HealthErrorCategoryTLS:
		message = "The MySQL TLS connection failed. Verify the datasource TLS settings and server certificate."
	case HealthErrorCategoryServer:
		message = "MySQL rejected the connection. Check server availability, capacity, and server logs."
	default:
		category = HealthErrorCategoryUnknown
		message = "The MySQL connection failed. Review the datasource configuration and MySQL server logs."
	}

	var mysqlErr *mysql.MySQLError
	hasMySQLError := errors.As(err, &mysqlErr)
	if hasMySQLError && mysqlErr == nil {
		return fmt.Sprintf("[%s] %s", category, message)
	}
	if hasMySQLError && mysqlErr.Number > 0 {
		message += fmt.Sprintf(" MySQL error number: %d.", mysqlErr.Number)
	}

	switch {
	case errors.Is(err, mysql.ErrCleartextPassword):
		message += " The account requires cleartext authentication; enable \"Allow Cleartext Passwords\" only when the connection is appropriately secured."
	case errors.Is(err, syscall.ECONNREFUSED):
		message += " MySQL refused the connection; verify that it is running and accepting connections on the configured port."
	case errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.EHOSTUNREACH):
		message += " The configured destination is unreachable from the Grafana server."
	case errors.Is(err, io.EOF):
		message += " The remote endpoint closed the connection during connection setup."
	}

	var addressErr *net.AddrError
	if errors.As(err, &addressErr) {
		message += " The server address is malformed; enter it as host:port without an http://, https://, or mysql:// prefix."
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		message += " The configured hostname or service name could not be resolved."
	}

	return fmt.Sprintf("[%s] %s", category, message)
}
