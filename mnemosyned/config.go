package main

import "flag"

// configuration ...
type configuration struct {
	host      string
	port      int
	subsystem string
	logger    struct {
		adapter string
		format  string
		level   int
	}
	storage struct {
		engine   string
		postgres struct {
			connectionString string
			tableName        string
			retry            int
		}
	}
}

// Init ...
func (c *configuration) Init() {
	if c == nil {
		*c = configuration{}
	}

	flag.StringVar(&c.host, "h", "127.0.0.1", "host")
	flag.IntVar(&c.port, "p", 8080, "port")
	flag.StringVar(&c.subsystem, "s", "mnemosyne", "subsystem")
	flag.StringVar(&c.logger.adapter, "la", loggerAdapterStdOut, "logger adapter")
	flag.StringVar(&c.logger.format, "lf", loggerFormatJSON, "logger format")
	flag.IntVar(&c.logger.level, "ll", 6, "logger level")
	flag.StringVar(&c.storage.engine, "se", storageEnginePostgres, "storage engine") // TODO: change to in memory when implemented
	flag.StringVar(&c.storage.postgres.connectionString, "spcs", "postgres://localhost:5432?sslmode=disable", "storage postgres connection string")
	flag.StringVar(&c.storage.postgres.tableName, "sptn", "mnemosyne_session", "storage postgres table name")
	flag.IntVar(&c.storage.postgres.retry, "spr", 10, "storage postgres possible attempts")
}

// Parse ...
func (c *configuration) Parse() {
	if !flag.Parsed() {
		flag.Parse()
	}
}
