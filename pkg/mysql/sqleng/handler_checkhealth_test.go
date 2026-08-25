package sqleng

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/handlertest"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/stretchr/testify/require"
)

func TestAdminHealthCheckResultIncludesVerboseMySQLErrorDetails(t *testing.T) {
	errWithDetails := &mysql.MySQLError{Number: 1045, Message: "example-user invalid-host"}

	result, err := ErrToHealthCheckResult(errWithDetails, HealthErrorCategoryAuth)

	require.NoError(t, err)
	require.Equal(t, backend.HealthStatusError, result.Status)
	require.Equal(t, healthErrorMessage(errWithDetails, HealthErrorCategoryAuth), result.Message)
	require.NotContains(t, result.Message, "example-user")
	require.NotContains(t, result.Message, "invalid-host")

	var details map[string]string
	require.NoError(t, json.Unmarshal(result.JSONDetails, &details))
	require.Equal(t, map[string]string{
		"errorDetailsLink": mysqlErrorDocsURL,
		"verboseMessage":   "Error 1045: example-user invalid-host",
	}, details)
}

type healthTestDriver struct {
	pingErr error
}

func (d healthTestDriver) Open(string) (driver.Conn, error) {
	return &healthTestConn{pingErr: d.pingErr}, nil
}

type healthTestConn struct {
	pingErr error
}

func (c *healthTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}
func (c *healthTestConn) Close() error               { return nil }
func (c *healthTestConn) Begin() (driver.Tx, error)  { return nil, errors.New("not implemented") }
func (c *healthTestConn) Ping(context.Context) error { return c.pingErr }

var healthTestDriverID uint64

func newHealthTestDB(t *testing.T, pingErr error) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("mysql-health-test-%d", atomic.AddUint64(&healthTestDriverID, 1))
	sql.Register(name, healthTestDriver{pingErr: pingErr})
	db, err := sql.Open(name, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func TestCheckHealthReturnsVerboseDetailsForAdminsAndFallbackForOthers(t *testing.T) {
	pingErr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: fmt.Errorf("invalid-host:3306: %w", syscall.ECONNREFUSED),
	}
	const userFacingError = "See internal runbook DB-12."
	wantMessage := healthErrorMessage(pingErr, HealthErrorCategoryNetwork)
	tests := []struct {
		name        string
		user        *backend.User
		wantDetails bool
		wantMessage string
	}{
		{name: "Admin", user: &backend.User{Role: "Admin"}, wantDetails: true, wantMessage: wantMessage},
		{name: "Viewer", user: &backend.User{Role: "Viewer"}, wantMessage: wantMessage + " " + userFacingError},
		{name: "missing user", wantMessage: wantMessage + " " + userFacingError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &DataSourceHandler{
				db:        newHealthTestDB(t, pingErr),
				log:       log.NewNullLogger(),
				userError: userFacingError,
			}
			result, err := handler.CheckHealth(context.Background(), &backend.CheckHealthRequest{
				PluginContext: backend.PluginContext{User: tt.user},
			})

			require.NoError(t, err)
			require.Equal(t, backend.HealthStatusError, result.Status)
			require.Equal(t, tt.wantMessage, result.Message)
			if !tt.wantDetails {
				require.Nil(t, result.JSONDetails)
				return
			}

			var details map[string]string
			require.NoError(t, json.Unmarshal(result.JSONDetails, &details))
			require.Equal(t, map[string]string{
				"errorDetailsLink": mysqlConfigurationDocsURL,
				"verboseMessage":   pingErr.Error(),
			}, details)
		})
	}
}

func TestCheckHealthReturnsOKForSuccessfulPing(t *testing.T) {
	handler := &DataSourceHandler{db: newHealthTestDB(t, nil), log: log.NewNullLogger()}

	result, err := handler.CheckHealth(context.Background(), &backend.CheckHealthRequest{})

	require.NoError(t, err)
	require.Equal(t, &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: "Database Connection OK",
	}, result)
}

func TestCheckHealthMarksPingFailuresAsDownstream(t *testing.T) {
	handler := &DataSourceHandler{
		db:  newHealthTestDB(t, errors.New("ping failed")),
		log: log.NewNullLogger(),
	}
	middlewareTest := handlertest.NewHandlerMiddlewareTest(t)
	var checkHealthContext context.Context
	middlewareTest.TestHandler.CheckHealthFunc = func(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
		checkHealthContext = ctx
		return handler.CheckHealth(ctx, req)
	}

	result, err := middlewareTest.MiddlewareHandler.CheckHealth(context.Background(), &backend.CheckHealthRequest{
		PluginContext: backend.PluginContext{User: &backend.User{Role: "Viewer"}},
	})

	require.NoError(t, err)
	require.Equal(t, backend.HealthStatusError, result.Status)
	require.Equal(t, backend.ErrorSourceDownstream, backend.ErrorSourceFromContext(checkHealthContext))
}
