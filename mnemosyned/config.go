package main

import (
	"flag"
	"fmt"
	"os"
)

const VERSION="0.0.1"

type configuration struct {
	host      string
	port      int
	namespace string
	subsystem string
	logger    struct {
		adapter string
		format  string
		level   int
	}
	monitoring struct {
		engine string
	}
	storage struct {
		engine   string
		postgres struct {
			connectionString string
			tableName        string
		}
	}
}

func (c *configuration) init() {
	if c == nil {
		*c = configuration{}
	}

	flag.StringVar(&c.host, "host", "127.0.0.1", "host")
	flag.IntVar(&c.port, "port", 8080, "port")
	flag.StringVar(&c.namespace, "namespace", "", "namespace")
	flag.StringVar(&c.subsystem, "subsystem", "mnemosyne", "subsystem")
	flag.StringVar(&c.logger.adapter, "l.adapter", loggerAdapterStdOut, "logger adapter")
	flag.StringVar(&c.logger.format, "l.format", loggerFormatJSON, "logger format")
	flag.IntVar(&c.logger.level, "l.level", 6, "logger level")
	flag.StringVar(&c.monitoring.engine, "m.engine", monitoringEnginePrometheus, "monitoring engine")
	flag.StringVar(&c.storage.engine, "s.engine", storageEnginePostgres, "storage engine") // TODO: change to in memory when implemented
	flag.StringVar(&c.storage.postgres.connectionString, "sp.connectionstring", "postgres://localhost:5432?sslmode=disable", "storage postgres connection string")
	flag.StringVar(&c.storage.postgres.tableName, "sp.tablename", "mnemosyne_session", "storage postgres table name")
}

func (c *configuration) parse() {
	ver := flag.Bool("version", false, "Print version and exit")
	if !flag.Parsed() {
		flag.Parse()
	}
	if *ver {
		fmt.Printf("%s", VERSION)
		os.Exit(0)
	}
}
