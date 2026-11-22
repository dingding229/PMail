package db

import (
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// IsUniqueViolation identifies a duplicate key without classifying other
// integrity violations (foreign keys, NOT NULL, CHECK) as conflicts.
func IsUniqueViolation(err error) bool {
	var pg *pq.Error
	if errors.As(err, &pg) && pg.Code == "23505" {
		return true
	}
	var my *mysql.MySQLError
	if errors.As(err, &my) && my.Number == 1062 {
		return true
	}
	var lite *sqlite.Error
	if !errors.As(err, &lite) {
		return false
	}
	switch lite.Code() {
	case sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY, sqlite3.SQLITE_CONSTRAINT_ROWID:
		return true
	case sqlite3.SQLITE_CONSTRAINT:
		// Accommodate SQLite errors carrying only the primary result code.
		return strings.Contains(strings.ToUpper(lite.Error()), "UNIQUE CONSTRAINT FAILED")
	}
	return false
}
