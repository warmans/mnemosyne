// +build postgres !unit

package main

import (
	"os"
	"testing"

	"github.com/piotrkowalczuk/sklog"
)

var (
	store Storage
)

func TestMain(m *testing.M) {
	config.parse()

	configPostgres := config.storage.postgres
	configLogger := config.logger

	logger := initLogger(configLogger.adapter, configLogger.format, configLogger.level, sklog.KeySubsystem, "mnemosyne")
	postgres := initPostgres(configPostgres.connectionString, logger)
	monitor := initMonitoring(initPrometheus(config.namespace, config.subsystem, nil), logger)
	store = initStorage(initPostgresStorage(configPostgres.tableName, postgres, monitor), logger)

	code := m.Run()

	store.TearDown()
	postgres.Close()

	os.Exit(code)
}

func TestPostgresStorage_Start(t *testing.T) {
	testStorage_Start(t, store)
}

func TestPostgresStorage_Get(t *testing.T) {
	testStorage_Get(t, store)
}

func TestPostgresStorage_List(t *testing.T) {
	testStorage_List(t, store)
}

func TestPostgresStorage_Exists(t *testing.T) {
	testStorage_Exists(t, store)
}

func TestPostgresStorage_Abandon(t *testing.T) {
	testStorage_Abandon(t, store)
}

func TestPostgresStorage_SetValue(t *testing.T) {
	testStorage_SetValue(t, store)
}

func TestPostgresStorage_Delete(t *testing.T) {
	testStorage_Delete(t, store)
}
