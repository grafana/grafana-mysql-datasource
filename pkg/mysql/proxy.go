package mysql

import (
	"context"
	"crypto/md5"
	"fmt"
	"net"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/net/proxy"

	"github.com/grafana/grafana-mysql-datasource/pkg/mysql/sqleng"
)

// createProxyIdentifier returns a hashed identifier that is unique per connection, stack and socks proxy
// network. The identifier is hashed so that it is safe to use as part of a network name, and so that it does
// not leak the connection details it is built from.
func createProxyIdentifier(dsInfo sqleng.DataSourceInfo, stackID string, secureDSProxyUsername string) string {
	socksProxyNetworkID := secureDSProxyUsername
	if socksProxyNetworkID == "" {
		socksProxyNetworkID = dsInfo.UID
	}
	identifier := dsInfo.User + dsInfo.DecryptedSecureJSONData["password"] + dsInfo.URL + dsInfo.Database + stackID + socksProxyNetworkID
	return fmt.Sprintf("proxy-%x", md5.Sum([]byte(identifier)))
}

// registerProxyDialerContext registers a new dialer context to be used by mysql when the proxy network is
// specified in the connection string
func registerProxyDialerContext(protocol, proxyIdentifier string, dialer proxy.Dialer) (string, error) {
	// the mysqlDialer contains the true network used behind the scenes
	mysqlDialer, err := getProxyDialerContext(protocol, dialer)
	if err != nil {
		return "", err
	}

	mysql.RegisterDialContext(proxyIdentifier, mysqlDialer.DialContext)

	return proxyIdentifier, nil
}

// mySQLContextDialer turns a golang proxy driver into a MySQL proxy driver
type mySQLContextDialer struct {
	dialer  proxy.ContextDialer
	network string
}

// getProxyDialerContext returns a context dialer that will send the request through to the secure socks proxy
func getProxyDialerContext(actualNetwork string, dialer proxy.Dialer) (*mySQLContextDialer, error) {
	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("mysql proxy creation failed")
	}
	return &mySQLContextDialer{dialer: contextDialer, network: actualNetwork}, nil
}

// DialContext implements the MySQL requirements for a proxy driver, and uses the underlying golang proxy driver with the assigned network
func (d *mySQLContextDialer) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	return d.dialer.DialContext(ctx, d.network, addr)
}
