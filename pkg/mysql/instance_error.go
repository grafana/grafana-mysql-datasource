package mysql

import (
	"context"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend"

	"github.com/grafana/grafana-mysql-datasource/pkg/mysql/sqleng"
)

const instanceConfigErrorMessage = "The MySQL datasource configuration is invalid. Review the datasource settings and Grafana server configuration."
const instanceTLSErrorMessage = "The MySQL TLS configuration is invalid. Verify the CA certificate, client certificate, and client key settings."

func categorizedInstanceError(ctx context.Context, category sqleng.HealthErrorCategory, message string) error {
	_ = backend.WithDownstreamErrorSource(ctx)
	return fmt.Errorf("[%s] %s", category, message)
}
