package schema_test

import (
	_ "embed"
	"reflect"
	"testing"

	"github.com/grafana/dsconfig/schema"
	"github.com/grafana/grafana-mysql-datasource/pkg/mysql/sqleng"
	"github.com/grafana/grafana-plugin-sdk-go/experimental/pluginschema"
	"k8s.io/kube-openapi/pkg/spec3"
)

//go:embed dsconfig.json
var configSchemaJSON []byte

// The loader uses a common SQL struct with fields for other database drivers.
// Keep its MySQL fields and add the keys that the frontend and Go SDK read.
func jsonDataModel() any {
	type editorAndSDKSettings struct {
		MaxIdleConnsAuto  bool    `json:"maxIdleConnsAuto"`
		TLSAuth           bool    `json:"tlsAuth"`
		TLSAuthWithCACert bool    `json:"tlsAuthWithCACert"`
		TLSServerName     string  `json:"serverName"`
		Timeout           float64 `json:"timeout"`
		KeepAlive         float64 `json:"keepAlive"`
	}
	fields := reflect.VisibleFields(reflect.TypeFor[editorAndSDKSettings]())
	for _, field := range reflect.VisibleFields(reflect.TypeFor[sqleng.JsonData]()) {
		switch field.Tag.Get("json") {
		case "connectionTimeout", "timescaledb", "sslmode", "tlsConfigurationMethod", "sslRootCertFile",
			"sslCertFile", "sslKeyFile", "encrypt", "servername", "authenticationType":
			continue
		}
		fields = append(fields, field)
	}
	return reflect.New(reflect.StructOf(fields)).Interface()
}

//go:generate go test -run TestPlugin -generateArtifacts
func TestPlugin(t *testing.T) {
	schema.RunPluginTests(t, schema.PluginUnderTest{
		ID:                "mysql",
		ConfigSchemaJSON:  configSchemaJSON,
		SettingsJSONModel: jsonDataModel(),
		SecureKeys:        []string{"password", "tlsCACert", "tlsClientCert", "tlsClientKey", "secureSocksProxyPassword"},
		SettingsExamples: &pluginschema.SettingsExamples{Examples: map[string]*spec3.Example{
			"":           example("MySQL over TCP", "localhost:3306", nil, "password"),
			"unixSocket": example("Passwordless local account over a Unix socket", "/var/run/mysqld/mysqld.sock", nil),
			"privateCA":  example("Verify the server with a private CA", "localhost:3306", []string{"tlsAuthWithCACert"}, "password", "tlsCACert"),
			"clientTLS":  example("Mutual TLS with system trust roots", "localhost:3306", []string{"tlsAuth"}, "password", "tlsClientCert", "tlsClientKey"),
			"clientTLSWithCA": example("Mutual TLS with a private CA", "localhost:3306", []string{"tlsAuth", "tlsAuthWithCACert"},
				"password", "tlsClientCert", "tlsClientKey", "tlsCACert"),
			"skipVerify":     example("TLS without server certificate verification", "localhost:3306", []string{"tlsSkipVerify"}, "password"),
			"cleartextAuth":  example("Cleartext password authentication over verified TLS", "localhost:3306", []string{"allowCleartextPasswords", "tlsAuthWithCACert"}, "password", "tlsCACert"),
			"privateNetwork": example("TCP through Private Data Source Connect", "mysql.internal:3306", []string{"enableSecureSocksProxy"}, "password"),
		}},
	})
}

func example(summary, host string, switches []string, secrets ...string) *spec3.Example {
	jsonData := map[string]any{"database": "grafana"}
	for _, key := range switches {
		jsonData[key] = true
	}
	value := map[string]any{
		"apiVersion": "mysql.datasource.grafana.app/v0alpha1",
		"kind":       "DataSource",
		"metadata":   map[string]string{"name": "mysql"},
		"spec":       map[string]any{"title": "MySQL", "url": host, "user": "grafana", "jsonData": jsonData},
	}
	if len(secrets) > 0 {
		placeholders := map[string]string{
			"password": "${GRAFANA_MYSQL_PASSWORD}", "tlsCACert": "${GRAFANA_TLS_CA_CERT}",
			"tlsClientCert": "${GRAFANA_TLS_CLIENT_CERT}", "tlsClientKey": "${GRAFANA_TLS_CLIENT_KEY}",
		}
		secure := map[string]map[string]string{}
		for _, key := range secrets {
			secure[key] = map[string]string{"create": placeholders[key]}
		}
		value["secure"] = secure
	}
	return &spec3.Example{ExampleProps: spec3.ExampleProps{Summary: summary, Value: value}}
}
