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

// createProxyIdentifier returns an identifier that is unique per connection, stack and PDC network.
// UID is only unique per org, the only way to ensure uniqueness is to do it by connection information.
// The stackID is the request namespace (e.g. "stacks-<id>"). The PDC network is selected by the secure
// socks proxy username, which defaults to the datasource UID, matching the SDK's ProxyOptions.
func createProxyIdentifier(dsInfo sqleng.DataSourceInfo, stackID string, secureDSProxyUsername string) string {
	pdcNetworkID := secureDSProxyUsername
	if pdcNetworkID == "" {
		pdcNetworkID = dsInfo.UID
	}
	return dsInfo.User + dsInfo.DecryptedSecureJSONData["password"] + dsInfo.URL + dsInfo.Database + stackID + pdcNetworkID
}

// registerProxyDialerContext registers a new dialer context to be used by mysql when the proxy network is
// specified in the connection string
func registerProxyDialerContext(protocol, cnnstr string, dialer proxy.Dialer) (string, error) {
	// the mysqlDialer contains the true network used behind the scenes
	mysqlDialer, err := getProxyDialerContext(protocol, dialer)
	if err != nil {
		return "", err
	}

	// the dialer context can be updated everytime the datasource is updated
	// have a unique network per connection string
	hash := fmt.Sprintf("%x", md5.Sum([]byte(cnnstr)))
	network := "proxy-" + hash
	mysql.RegisterDialContext(network, mysqlDialer.DialContext)

	return network, nil
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
