package main

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/go-kit/kit/metrics"
	"github.com/piotrkowalczuk/mnemosyne"
	"github.com/piotrkowalczuk/protot"
)

const (
	postgresSchema = `
		CREATE SCHEMA IF NOT EXISTS mnemosyne;
		CREATE TABLE IF NOT EXISTS mnemosyne.session (
			token BYTEA PRIMARY KEY,
			subject_id TEXT NOT NULL,
			bag bytea NOT NULL,
			expire_at timestamp with time zone NOT NULL
		)
    `
)

var (
	tmpKey = []byte(hex.EncodeToString([]byte("1")))
)

type postgresStorage struct {
	db        *sql.DB
	tableName string
	generator mnemosyne.RandomBytesGenerator
	monitor   *monitoring
}

func newPostgresStorage(tn string, db *sql.DB, m *monitoring) Storage {
	return &postgresStorage{
		db:        db,
		tableName: tn,
		generator: &mnemosyne.SystemRandomBytesGenerator{},
		monitor:   m,
	}
}

func initPostgresStorage(tn string, db *sql.DB, m *monitoring) func() (Storage, error) {
	return func() (Storage, error) {
		return newPostgresStorage(tn, db, m), nil
	}
}

// Create implements Storage interface.
func (ps *postgresStorage) Start(subjectID string, bag map[string]string) (*mnemosyne.Session, error) {
	token, err := mnemosyne.RandomToken(ps.generator, tmpKey)
	if err != nil {
		return nil, err
	}

	entity := &sessionEntity{
		Token:     token,
		SubjectID: subjectID,
		Bag:       bagpack(bag),
	}

	if err := ps.save(entity); err != nil {
		return nil, err
	}

	return newSessionFromSessionEntity(entity), nil
}

func (ps *postgresStorage) save(entity *sessionEntity) (err error) {
	query := `
		INSERT INTO mnemosyne.session (token, subject_id, bag, expire_at)
		VALUES ($1, $2, $3, NOW() + '30 minutes'::interval)
		RETURNING expire_at

	`
	field := metrics.Field{Key: "query", Value: query}

	err = ps.db.QueryRow(
		query,
		entity.Token,
		entity.SubjectID,
		entity.Bag,
	).Scan(
		&entity.ExpireAt,
	)
	ps.monitor.postgres.queries.With(field).Add(1)

	return
}

// Get implements Storage interface.
func (ps *postgresStorage) Get(token *mnemosyne.Token) (*mnemosyne.Session, error) {
	var entity sessionEntity
	query := `
		SELECT subject_id, bag, expire_at
		FROM mnemosyne.session
		WHERE token = $1
		LIMIT 1
	`
	field := metrics.Field{Key: "query", Value: query}

	err := ps.db.QueryRow(query, token).Scan(
		&entity.SubjectID,
		&entity.Bag,
		&entity.ExpireAt,
	)
	if err != nil {
		ps.monitor.postgres.errors.With(field).Add(1)
		if err == sql.ErrNoRows {
			return nil, errSessionNotFound
		}
		return nil, err
	}

	return &mnemosyne.Session{
		Token:     token,
		SubjectId: entity.SubjectID,
		Bag:       entity.Bag,
		ExpireAt:  protot.TimeToTimestamp(entity.ExpireAt),
	}, nil
}

// List implements Storage interface.
func (ps *postgresStorage) List(offset, limit int64, expiredAtFrom, expiredAtTo *time.Time) ([]*mnemosyne.Session, error) {
	if limit == 0 {
		return nil, errors.New("mnemosyned: cannot retrieve list of sessions, limit needs to be higher than 0")
	}

	args := []interface{}{offset, limit}
	query := "SELECT token, subject_id, bag, expire_at FROM mnemosyne.session"

	switch {
	case expiredAtFrom != nil && expiredAtTo == nil:
		query += "expire_at > $3"
		args = append(args, expiredAtFrom)
	case expiredAtFrom == nil && expiredAtTo != nil:
		query += "expire_at < $3"
		args = append(args, expiredAtTo)
	case expiredAtFrom != nil && expiredAtTo != nil:
		query += "expire_at > $4 AND expire_at < $5"
		args = append(args, expiredAtFrom, expiredAtTo)
	}

	query += " OFFSET $1 LIMIT $2"

	field := metrics.Field{Key: "query", Value: query}

	rows, err := ps.db.Query(query, args...)
	if err != nil {
		ps.monitor.postgres.errors.With(field).Add(1)
		return nil, err
	}
	defer rows.Close()

	ps.monitor.postgres.queries.With(field).Add(1)

	sessions := make([]*mnemosyne.Session, 0, limit)
	for rows.Next() {
		var entity sessionEntity

		err = rows.Scan(
			&entity.Token,
			&entity.SubjectID,
			&entity.Bag,
			&entity.ExpireAt,
		)
		if err != nil {
			ps.monitor.postgres.errors.With(field).Add(1)
			return nil, err
		}

		sessions = append(sessions, &mnemosyne.Session{
			Token:     &entity.Token,
			SubjectId: entity.SubjectID,
			Bag:       entity.Bag,
			ExpireAt:  protot.TimeToTimestamp(entity.ExpireAt),
		})
	}
	if rows.Err() != nil {
		ps.monitor.postgres.errors.With(field).Add(1)
		return nil, rows.Err()
	}

	return sessions, nil
}

// Exists implements Storage interface.
func (ps *postgresStorage) Exists(token *mnemosyne.Token) (exists bool, err error) {
	query := `SELECT EXISTS(SELECT 1 FROM mnemosyne.session WHERE token = $1)`
	field := metrics.Field{Key: "query", Value: query}

	err = ps.db.QueryRow(query, *token).Scan(
		&exists,
	)
	if err != nil {
		ps.monitor.postgres.errors.With(field).Add(1)
	}
	ps.monitor.postgres.queries.With(field).Add(1)

	return
}

// Abandon ...
func (ps *postgresStorage) Abandon(token *mnemosyne.Token) (bool, error) {
	query := `DELETE FROM mnemosyne.session WHERE token = $1`
	field := metrics.Field{Key: "query", Value: query}

	result, err := ps.db.Exec(query, *token)
	if err != nil {
		ps.monitor.postgres.errors.With(field).Add(1)
		return false, err
	}

	ps.monitor.postgres.queries.With(field).Add(1)

	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	if affected == 0 {
		return false, errSessionNotFound
	}

	return true, nil
}

// SetData implements Storage interface.
func (ps *postgresStorage) SetValue(token *mnemosyne.Token, key, value string) (map[string]string, error) {
	var err error

	entity := &sessionEntity{
		Token: *token,
	}
	selectQuery := `
		SELECT subject_id, bag, expire_at
		FROM mnemosyne.session
		WHERE token = $1
		FOR UPDATE
	`
	updateQuery := `
		UPDATE mnemosyne.session
		SET
			bag = $2
		WHERE token = $1
	`

	tx, err := ps.db.Begin()
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	err = tx.QueryRow(selectQuery, *token).Scan(
		&entity.SubjectID,
		&entity.Bag,
		&entity.ExpireAt,
	)
	if err != nil {
		ps.monitor.postgres.errors.With(metrics.Field{Key: "query", Value: selectQuery}).Add(1)
		tx.Rollback()
		if err == sql.ErrNoRows {
			return nil, errSessionNotFound
		}
		return nil, err
	}
	ps.monitor.postgres.queries.With(metrics.Field{Key: "query", Value: selectQuery}).Add(1)

	entity.Bag.Set(key, value)

	_, err = tx.Exec(updateQuery, *token, entity.Bag)
	if err != nil {
		ps.monitor.postgres.errors.With(metrics.Field{Key: "query", Value: updateQuery}).Add(1)
		tx.Rollback()
		return nil, err
	}
	ps.monitor.postgres.queries.With(metrics.Field{Key: "query", Value: updateQuery}).Add(1)

	tx.Commit()

	return entity.Bag, nil
}

// Delete implements Storage interface.
func (ps *postgresStorage) Delete(token *mnemosyne.Token, expiredAtFrom, expiredAtTo *time.Time) (int64, error) {
	if token == nil && expiredAtFrom == nil && expiredAtTo == nil {
		return 0, errors.New("mnemosyned: session cannot be deleted, no where parameter provided")
	}

	where, args := ps.where(token, expiredAtFrom, expiredAtTo)
	query := "DELETE FROM mnemosyne.session WHERE " + where
	field := metrics.Field{Key: "query", Value: query}

	result, err := ps.db.Exec(query, args...)
	if err != nil {
		ps.monitor.postgres.errors.With(field).Add(1)
		return 0, err
	}
	ps.monitor.postgres.queries.With(field).Add(1)

	return result.RowsAffected()
}

// Setup implements Storage interface.
func (ps *postgresStorage) Setup() error {
	_, err := ps.db.Exec(postgresSchema)

	return err
}

// TearDown implements Storage interface.
func (ps *postgresStorage) TearDown() error {
	_, err := ps.db.Exec(`DROP SCHEMA mnemosyne`)

	return err
}

func (ps *postgresStorage) where(token *mnemosyne.Token, expiredAtFrom, expiredAtTo *time.Time) (string, []interface{}) {
	switch {
	case token != nil && expiredAtFrom == nil && expiredAtTo == nil:
		return "token = $1", []interface{}{token}
	case token == nil && expiredAtFrom != nil && expiredAtTo == nil:
		return "expire_at > $1", []interface{}{expiredAtFrom}
	case token == nil && expiredAtFrom == nil && expiredAtTo != nil:
		return "expire_at < $1", []interface{}{expiredAtTo}
	case token != nil && expiredAtFrom != nil && expiredAtTo == nil:
		return "token = $1 AND expire_at > $2", []interface{}{token, expiredAtFrom}
	case token != nil && expiredAtFrom == nil && expiredAtTo != nil:
		return "token = $1 AND expire_at < $2", []interface{}{token, expiredAtTo}
	case token == nil && expiredAtFrom != nil && expiredAtTo != nil:
		return "expire_at > $1 AND expire_at < $2", []interface{}{expiredAtFrom, expiredAtTo}
	case token != nil && expiredAtFrom != nil && expiredAtTo != nil:
		return "token = $1 AND expire_at > $2 AND expire_at < $3", []interface{}{token, expiredAtFrom, expiredAtTo}
	default:
		return "", nil
	}
}

type sessionEntity struct {
	Token     mnemosyne.Token `json:"token"`
	SubjectID string          `json:"subjectId"`
	Bag       bagpack         `json:"bag"`
	ExpireAt  time.Time       `json:"expireAt"`
}

func newSessionFromSessionEntity(entity *sessionEntity) *mnemosyne.Session {
	return &mnemosyne.Session{
		Token:     &entity.Token,
		SubjectId: entity.SubjectID,
		Bag:       entity.Bag,
		ExpireAt:  protot.TimeToTimestamp(entity.ExpireAt),
	}
}
