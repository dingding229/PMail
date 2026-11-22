package db

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
)

func TestIsUniqueViolationAcrossDrivers(t *testing.T) {
	for _, tc := range []struct {
		err    error
		unique bool
	}{
		{fmt.Errorf("insert domain: %w", &pq.Error{Code: "23505"}), true},
		{&pq.Error{Code: "23503"}, false},
		{fmt.Errorf("insert address: %w", &mysql.MySQLError{Number: 1062}), true},
		{&mysql.MySQLError{Number: 1048}, false},
		{fmt.Errorf("untyped UNIQUE constraint failed"), false},
		{nil, false},
	} {
		if got := IsUniqueViolation(tc.err); got != tc.unique {
			t.Errorf("%T (%v): unique=%v want %v", tc.err, tc.err, got, tc.unique)
		}
	}
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)
	for _, query := range []string{`CREATE TABLE records (id INTEGER PRIMARY KEY, name TEXT UNIQUE NOT NULL)`, `INSERT INTO records VALUES (1,'one')`} {
		if _, err := conn.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		query  string
		unique bool
	}{
		{`INSERT INTO records VALUES (2,'one')`, true},
		{`INSERT INTO records VALUES (1,'two')`, true},
		{`INSERT INTO records VALUES (3,NULL)`, false},
	} {
		if _, err := conn.Exec(tc.query); err == nil || IsUniqueViolation(fmt.Errorf("fixture: %w", err)) != tc.unique {
			t.Errorf("%s: unexpected constraint classification: %v", tc.query, err)
		}
	}
}
