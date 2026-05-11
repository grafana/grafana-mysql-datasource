package main

import (
	"os"

	"github.com/grafana/grafana-mysql-datasource/pkg/mysql"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

func main() {
	logger := backend.NewLoggerWith("logger", "tsdb.mysql")
	if err := datasource.Manage("mysql", mysql.NewInstanceSettings(logger), datasource.ManageOpts{}); err != nil {
		log.DefaultLogger.Error(err.Error())
		os.Exit(1)
	}
}
