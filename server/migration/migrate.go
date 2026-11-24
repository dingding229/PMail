package migration

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
	"modernc.org/sqlite"
)

// Required tables are present in the oldest supported source. The rest
// may not exist yet; do not confuse an absent source with a target error.
var requiredTables = []string{"user", "email", "group", "rule", "user_email", "version"}
var optionalTables = []string{
	"domain", "mailbox_address", "address_settings", "api_token",
	"totp_config", "recovery_code", "app_password",
	"user_email_address", "user_email_address_progress", "imap_mailbox", "imap_mailbox_uid",
	"forward_setting", "outbound_payload", "outbound_queue", "bounce_notice", "inbound_settings", "trash_settings",
	"spam_preference", "spam_rule", "spam_training", "spam_totals", "spam_token",
	"telegram_config", "telegram_binding", "telegram_code", "telegram_preference", "telegram_push",
	"compose_image", "draft_inline_image", "signature", "translation_settings", "user_ai_config",
	"translation_preference", "translation_usage", "email_translation", "email_ai_result",
}
var tables = slices.Concat(requiredTables, optionalTables)
var errSourceTableAbsent = errors.New("source table absent")

const batchSize = 200

func Run(args []string) error {
	fs := flag.NewFlagSet("pmail migrate", flag.ContinueOnError)
	fromType := fs.String("from-type", "", "source database type: sqlite or mysql")
	fromDSN := fs.String("from-dsn", "", "source database DSN")
	force := fs.Bool("force", false, "replace all target tables, discarding existing data and sessions")
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return err
	}
	if fs.NArg() != 0 || (*fromType != config.DBTypeSQLite && *fromType != config.DBTypeMySQL) || *fromDSN == "" {
		return errors.New("usage: pmail migrate --from-type sqlite|mysql --from-dsn <dsn> [--force]")
	}
	cfg, err := config.ReadEffectiveConfig()
	if err != nil {
		return err
	}
	if cfg.DbType != config.DBTypePostgres {
		return errors.New("migration target must use PostgreSQL")
	}

	source, sourceTx, err := openSource(*fromType, *fromDSN)
	if err != nil {
		return err
	}
	defer source.Close()
	defer sourceTx.Rollback()

	engine, err := db.Open(cfg, true)
	if err != nil {
		return err
	}
	defer engine.Close()
	target := engine.DB().DB
	schema := engine.Dialect().URI().Schema
	if schema == "" {
		schema = "public"
	}
	qualified := func(table string) string { return pq.QuoteIdentifier(schema) + "." + pq.QuoteIdentifier(table) }
	if !*force {
		// Do not run Sync2 on an existing non-empty target. Repeat the guard under a lock below.
		for _, table := range tables {
			var exists bool
			if err := target.QueryRow("SELECT to_regclass($1) IS NOT NULL", qualified(table)).Scan(&exists); err != nil {
				return fmt.Errorf("inspect PostgreSQL table %s: %w", table, err)
			}
			if exists {
				if err := requireEmpty(target, qualified(table)); err != nil {
					return err
				}
			}
		}
	}
	if err := db.SyncSchema(engine); err != nil {
		return err
	}

	tx, err := target.Begin()
	if err != nil {
		return fmt.Errorf("begin PostgreSQL transaction: %w", err)
	}
	defer tx.Rollback()
	var names []string
	for _, table := range tables {
		names = append(names, qualified(table))
	}
	// Sessions are not imported. Clearing them prevents old tokens from being linked to imported IDs.
	names = append(names, qualified("sessions"), qualified("mail_attachment_info"), qualified("mail_attachment_chunk"))
	list := strings.Join(names, ", ")
	if _, err := tx.Exec("LOCK TABLE " + list + " IN ACCESS EXCLUSIVE MODE"); err != nil {
		return fmt.Errorf("lock PostgreSQL tables: %w", err)
	}
	if !*force {
		for _, table := range tables {
			if err := requireEmpty(tx, qualified(table)); err != nil {
				return err
			}
		}
	} else {
		fmt.Println("--force: replacing target data and clearing sessions")
	}
	// RESTART IDENTITY also makes sequence resets on these sequences roll back with the transaction.
	if _, err := tx.Exec("TRUNCATE TABLE " + list + " RESTART IDENTITY"); err != nil {
		return fmt.Errorf("truncate PostgreSQL tables: %w", err)
	}

	for _, table := range tables {
		if err := copyTable(sourceTx, tx, *fromType, table, qualified(table)); err != nil {
			if slices.Contains(optionalTables, table) && errors.Is(err, errSourceTableAbsent) {
				fmt.Printf("%s: source table absent; normal startup will backfill it\n", table)
				continue
			}
			return fmt.Errorf("copy %s: %w", table, err)
		}
	}
	// Attachment bytes and POP octets are derived from retained legacy mail.
	if _, err := tx.Exec("UPDATE " + qualified("email") + " SET projection_ready=false, pop_octets=0 WHERE projection_ready OR pop_octets<>0"); err != nil { // #nosec G202 -- fixed email table and PostgreSQL-quoted schema from the operator DSN.
		return err
	}
	for _, table := range tables {
		if err := resetSequence(tx, table, qualified(table)); err != nil {
			return fmt.Errorf("reset %s sequence: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit PostgreSQL transaction: %w", err)
	}
	fmt.Println("Migration committed successfully.")
	return nil
}

func openSource(driver, dsn string) (*sql.DB, *sql.Tx, error) {
	opts := &sql.TxOptions{ReadOnly: true}
	if driver == config.DBTypeSQLite {
		// The legacy SQLite DSN may be a file path with query parameters or a file: URI.
		// Open read-only so a typo never creates a new, empty source database.
		path, query, _ := strings.Cut(dsn, "?")
		if path == "" || path == ":memory:" || path == "file::memory:" {
			return nil, nil, errors.New("SQLite source must be an existing database file")
		}
		if !strings.HasPrefix(path, "file:") {
			abs, err := filepath.Abs(path)
			if err != nil {
				return nil, nil, err
			}
			path = (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String()
		}
		values, err := url.ParseQuery(query)
		if err != nil {
			return nil, nil, errors.New("invalid SQLite source DSN query")
		}
		values.Set("mode", "ro")
		dsn = path + "?" + values.Encode()
	} else {
		opts.Isolation = sql.LevelRepeatableRead
	}
	source, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("could not open %s source database; verify the source DSN", driver)
	}
	tx, err := source.BeginTx(context.Background(), opts)
	if err != nil {
		source.Close()
		return nil, nil, fmt.Errorf("open read-only %s transaction: %w", driver, err)
	}
	return source, tx, nil
}

type rowQuerier interface {
	QueryRow(string, ...any) *sql.Row
}

func requireEmpty(target rowQuerier, table string) error {
	var exists bool
	if err := target.QueryRow("SELECT EXISTS (SELECT 1 FROM " + table + ")").Scan(&exists); err != nil {
		return fmt.Errorf("inspect PostgreSQL table %s: %w", table, err)
	}
	if exists {
		return fmt.Errorf("target table %s is not empty; use --force only to replace existing data", table)
	}
	return nil
}

func copyTable(source, target *sql.Tx, driver, name, dest string) error {
	if !slices.Contains(tables, name) {
		return fmt.Errorf("unsupported migration table %q", name)
	}
	quoteSource := pq.QuoteIdentifier
	if driver == config.DBTypeMySQL {
		quoteSource = func(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }
	}
	from := quoteSource(name)
	cols, err := columns(source, from)
	if err != nil {
		if isMissingSourceTable(err) {
			return errSourceTableAbsent
		}
		return err
	}
	targetCols, err := columns(target, dest)
	if err != nil {
		return err
	}
	var sourceNames, targetNames, params []string
	idColumn := migrationKeyColumn(name)
	binary := map[int]bool{}
	idIndex := -1
	for i, col := range cols {
		if !slices.Contains(targetCols, col) {
			return fmt.Errorf("source column %q is missing from PostgreSQL; refusing to drop data", col)
		}
		if col == "wire_data" || col == "raw_data" || (name == "compose_image" && col == "content") {
			binary[i] = true
		}
		if col == idColumn {
			idIndex = i
		}
		sourceNames = append(sourceNames, quoteSource(col))
		targetNames = append(targetNames, pq.QuoteIdentifier(col))
		params = append(params, fmt.Sprintf("$%d", i+1))
	}
	if idIndex == -1 && idColumn != "" {
		return fmt.Errorf("missing source migration key column %q", idColumn)
	}
	insert, err := target.Prepare(fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", dest,
		strings.Join(targetNames, ", "), strings.Join(params, ", ")))
	if err != nil {
		return err
	}
	defer insert.Close()

	selectSQL := "SELECT " + strings.Join(sourceNames, ", ") + " FROM " + from // #nosec G202 -- the table is allowlisted; columns must exist in both schemas and every identifier is quoted for its dialect.
	if idColumn == "" {
		// Composite/hashed keys: stream rows between independent database
		// transactions. Do not put the full training vocabulary in memory.
		rows, err := source.Query(selectSQL)
		if err != nil {
			return err
		}
		n, err := copyRows(rows, insert, len(cols), -1, nil, binary)
		if err == nil {
			fmt.Printf("%s: %d rows copied (pending commit)\n", name, n)
		}
		return err
	}
	order := " ORDER BY " + quoteSource(idColumn) + " LIMIT ?"
	var copied, lastID int64
	for {
		var rows *sql.Rows
		if copied == 0 {
			rows, err = source.Query(selectSQL+order, batchSize)
		} else {
			rows, err = source.Query(selectSQL+" WHERE "+quoteSource(idColumn)+" > ?"+order, lastID, batchSize)
		}
		if err != nil {
			return err
		}
		n, err := copyRows(rows, insert, len(cols), idIndex, &lastID, binary)
		if err != nil {
			return err
		}
		copied += int64(n)
		if n > 0 || copied == 0 {
			fmt.Printf("%s: %d rows copied (pending commit)\n", name, copied)
		}
		if n < batchSize {
			return nil
		}
	}
}

func columns(tx *sql.Tx, table string) ([]string, error) {
	rows, err := tx.Query("SELECT * FROM " + table + " WHERE 1 = 0") // #nosec G202 -- only called with dialect-quoted identifiers from the offline migration table allowlist.
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rows.Columns()
}

func copyRows(rows *sql.Rows, insert *sql.Stmt, width, idIndex int, lastID *int64, binary map[int]bool) (int, error) {
	defer rows.Close()
	values, pointers := make([]any, width), make([]any, width)
	for i := range values {
		pointers[i] = &values[i]
	}
	if idIndex >= 0 {
		pointers[idIndex] = lastID
	}
	count := 0
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return count, err
		}
		if idIndex >= 0 {
			values[idIndex] = *lastID
		}
		for i, value := range values {
			if b, ok := value.([]byte); ok && !binary[i] {
				// MySQL returns text/dates as bytes; keep binary queue payloads as bytes.
				values[i] = string(b)
			}
		}
		if _, err := insert.Exec(values...); err != nil {
			if idIndex >= 0 {
				return count, fmt.Errorf("insert id %d: %w", *lastID, err)
			}
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func migrationKeyColumn(table string) string {
	switch table {
	case "totp_config", "forward_setting", "spam_preference", "spam_totals", "telegram_binding", "telegram_code", "telegram_preference", "user_ai_config", "translation_preference", "translation_usage":
		return "user_id"
	case "recovery_code", "user_email_address", "imap_mailbox_uid", "spam_training", "spam_token", "compose_image", "draft_inline_image", "email_translation", "email_ai_result":
		return ""
	}
	return "id"
}

func isMissingSourceTable(err error) bool {
	var my *mysql.MySQLError
	var lite *sqlite.Error
	return (errors.As(err, &my) && my.Number == 1146) ||
		(errors.As(err, &lite) && lite.Code() == 1 && strings.Contains(lite.Error(), "no such table:"))
}

func hasIDSequence(table string) bool {
	if migrationKeyColumn(table) != "id" {
		return false
	}
	switch table {
	case "address_settings", "inbound_settings", "trash_settings", "telegram_config", "translation_settings", "user_email_address_progress":
		return false
	}
	return true
}

func resetSequence(tx *sql.Tx, table, qualified string) error {
	if !hasIDSequence(table) {
		return nil
	}
	var value int64
	return tx.QueryRow(`SELECT setval(s.seqrelid, greatest(coalesce(m.max_id, s.seqmin), s.seqmin),
		coalesce(m.max_id >= s.seqmin, false))
		FROM pg_sequence s CROSS JOIN (SELECT max(id) AS max_id FROM `+qualified+`) m
		WHERE s.seqrelid = pg_get_serial_sequence($1, 'id')::regclass`, qualified).Scan(&value)
}
