package session

import (
	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/alexedwards/scs/mysqlstore"
	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
	log "github.com/sirupsen/logrus"
	"net/http"

	"time"
)

var Instance *scs.SessionManager

func Init() {
	Instance = scs.New()
	Instance.ErrorFunc = func(w http.ResponseWriter, r *http.Request, err error) {
		log.WithContext(r.Context()).Errorf("Session middleware: %v", err)
		w.Header().Set("Content-Type", "application/json")
		response.Localized(i18n.ForRequest(r), response.ServerError, "internal_error").FPrintStatus(w, http.StatusInternalServerError)
	}
	Instance.Lifetime = 7 * 24 * time.Hour
	// 使用db存储session数据，目前为了架构简单，
	// 暂不引入redis存储，如果日后性能存在瓶颈，可以将session迁移到redis

	switch config.Instance.DbType {
	case config.DBTypeMySQL:
		Instance.Store = mysqlstore.New(db.Instance.DB().DB)
	case config.DBTypeSQLite:
		Instance.Store = sqlite3store.New(db.Instance.DB().DB)
	case config.DBTypePostgres:
		Instance.Store = postgresstore.New(db.Instance.DB().DB)
	default:
		panic("Unsupported database type: " + config.Instance.DbType)
	}

}

// Stop releases the cleanup goroutine before closing or replacing the database pool.
func Stop() {
	if Instance == nil {
		return
	}
	if store, ok := Instance.Store.(interface{ StopCleanup() }); ok {
		store.StopCleanup()
	}
}
