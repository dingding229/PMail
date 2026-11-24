package backupcatalog

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

// ResetPostgresSequence uses transactional RESTART (unlike setval). The
// database resolves the owned sequence and the names are individually quoted.
func ResetPostgresSequence(ctx context.Context, tx *sql.Tx, table string, last int64) error {
	var schema, name string
	if err := tx.QueryRowContext(ctx, `SELECT n.nspname,c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid=pg_get_serial_sequence($1, 'id')::regclass`, table).Scan(&schema, &name); err != nil {
		return err
	}
	if last < 0 || last == int64(^uint64(0)>>1) {
		return fmt.Errorf("invalid sequence watermark")
	}
	_, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER SEQUENCE %s.%s RESTART WITH %d", pq.QuoteIdentifier(schema), pq.QuoteIdentifier(name), last+1))
	return err
}
