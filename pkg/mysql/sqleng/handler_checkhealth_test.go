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

func TestCheckHealthRestrictsVerboseErrorDetailsToAdmins(t *testing.T) {
	pingErr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: fmt.Errorf("invalid-host:3306: %w", syscall.ECONNREFUSED),
	}
	results := make(map[string]*backend.CheckHealthResult)
	for _, role := range []string{"Admin", "Viewer"} {
		handler := &DataSourceHandler{
			db:  newHealthTestDB(t, pingErr),
			log: log.NewNullLogger(),
		}
		result, err := handler.CheckHealth(context.Background(), &backend.CheckHealthRequest{
			PluginContext: backend.PluginContext{User: &backend.User{Role: role}},
		})
		require.NoError(t, err)
		require.Equal(t, backend.HealthStatusError, result.Status)
		results[role] = result
	}

	wantMessage := healthErrorMessage(pingErr, HealthErrorCategoryNetwork)
	require.Equal(t, wantMessage, results["Admin"].Message)
	require.Equal(t, wantMessage, results["Viewer"].Message)
	require.Contains(t, string(results["Admin"].JSONDetails), "invalid-host")
	require.NotContains(t, string(results["Viewer"].JSONDetails), "invalid-host")

	var adminDetails map[string]string
	require.NoError(t, json.Unmarshal(results["Admin"].JSONDetails, &adminDetails))
	require.Equal(t, map[string]string{
		"errorDetailsLink": mysqlConfigurationDocsURL,
		"verboseMessage":   pingErr.Error(),
	}, adminDetails)
	require.Nil(t, results["Viewer"].JSONDetails)
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
