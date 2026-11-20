// Package testschema contains the shared SQLite fixture schema used by tests
// that exercise T2 domain/address behavior. Keeping it on top of the
// production schema list prevents individual packages from omitting new
// tables when fixtures are extended.
package testschema

import (
	"github.com/Jinnrry/pmail/db"
	"xorm.io/xorm"
)

// Sync applies the same complete schema as production to a test engine.
func Sync(engine *xorm.Engine) error {
	return db.SyncSchema(engine)
}
