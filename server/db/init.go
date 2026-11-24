package db

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/utils/context"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

var Instance *xorm.Engine

// MailboxRegistry serializes domain/address ownership mutations in the single-instance server.
var MailboxRegistry sync.Mutex

// Open connects without migrating schema or writing application data.
func Open(cfg *config.Config, retry bool) (*xorm.Engine, error) {
	dsn := cfg.DbDSN
	if dsn == "" {
		return nil, fmt.Errorf("database DSN is required for %s", cfg.DbType)
	}
	switch cfg.DbType {
	case config.DBTypeSQLite:
		var err error
		dsn, err = sqliteDSNWithReliabilityOptions(dsn)
		if err != nil {
			return nil, err
		}
	case config.DBTypePostgres:
		parsed, err := url.Parse(dsn)
		if err != nil || parsed.Scheme != "postgres" || parsed.Hostname() == "" ||
			strings.Trim(parsed.Path, "/") == "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("PostgreSQL DSN must be a postgres:// URL, e.g. postgres://user:pass@host:5432/db?sslmode=disable")
		}
	case config.DBTypeMySQL:
	default:
		return nil, fmt.Errorf("unsupported database type")
	}

	engine, err := xorm.NewEngine(cfg.DbType, dsn)
	if err != nil {
		// Some driver parsers include the original DSN in errors.
		return nil, fmt.Errorf("could not open %s database; verify the configured DSN", cfg.DbType)
	}
	if cfg.DbType == config.DBTypeSQLite {
		engine.SetMaxOpenConns(1)
		engine.SetMaxIdleConns(1)
	} else {
		engine.SetMaxOpenConns(100)
		engine.SetMaxIdleConns(10)
	}
	engine.SetConnMaxLifetime(30 * time.Minute)
	if err := pingWithRetry(engine, retry && cfg.DbType == config.DBTypePostgres); err != nil {
		engine.Close()
		return nil, fmt.Errorf("connect to %s database: %w", cfg.DbType, err)
	}
	return engine, nil
}

func InitStartup(version string, preserveUsers bool) error {
	engine, err := Open(config.Instance, true)
	if err != nil {
		return err
	}
	if err := initializeSchemaAndData(engine, version, preserveUsers); err != nil {
		engine.Close()
		return err
	}
	// Xorm SQL logging includes bind values, including password hashes and DKIM keys.
	engine.ShowSQL(false)
	if Instance != nil {
		Instance.Close()
	}
	Instance = engine
	return nil
}

func initializeSchemaAndData(engine *xorm.Engine, version string, preserveUsers bool) error {
	if err := SyncSchema(engine); err != nil {
		return err
	}
	if err := dropExternalMailboxTables(engine); err != nil {
		return err
	}
	if err := fixHistoryData(engine, preserveUsers); err != nil {
		return err
	}
	if err := backfillTrash(engine, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("backfill Trash timestamps: %w", err)
	}
	if version == "" || version == "test" {
		return nil
	}
	var v models.Version
	if _, err := engine.Get(&v); err != nil {
		return err
	}
	if v.Info == version {
		return nil
	}
	v.Info = version
	if v.Id == 0 {
		_, err := engine.Insert(&v)
		return err
	}
	_, err := engine.ID(v.Id).Cols("info").Update(&v)
	return err
}

// SyncSchema is also used by the offline migration command, which must not seed or repair data.
func SyncSchema(engine *xorm.Engine) error {
	for _, model := range []any{
		&models.User{}, &models.Email{}, &models.MailAttachmentInfo{}, &models.MailAttachmentChunk{}, &models.Group{}, &models.Rule{},
		&models.Sessions{}, &models.TOTPConfig{}, &models.RecoveryCode{}, &models.AppPassword{}, &models.UserEmail{}, &models.TrashSettings{}, &models.UserEmailAddress{}, &models.UserEmailAddressProgress{}, &models.IMAPMailbox{}, &models.IMAPMailboxUID{}, &models.Version{}, &models.Domain{}, &models.MailboxAddress{}, &models.AddressSettings{}, &models.APIToken{}, &models.OutboundPayload{}, &models.OutboundQueue{}, &models.ForwardSetting{}, &models.BounceNotice{}, &models.RegistrationGuard{},
		&models.InboundSettings{}, &models.SpamPreference{}, &models.SpamRule{}, &models.SpamTraining{}, &models.SpamTotals{}, &models.SpamToken{},
		&models.TranslationSettings{}, &models.AIConfig{}, &models.TranslationPreference{}, &models.TranslationUsage{}, &models.EmailTranslation{}, &models.EmailAIResult{},
		&models.TelegramConfig{}, &models.TelegramBinding{}, &models.TelegramCode{}, &models.TelegramPreference{}, &models.TelegramPush{}, &models.ComposeImage{}, &models.DraftInlineImage{}, &models.Signature{}, &models.MailArchiveSettings{},
		&models.ReportMailPreference{}, &models.DMARCReport{}, &models.DMARCRecord{}, &models.TLSReport{},
	} {
		var err error
		switch model := model.(type) {
		case *models.UserEmail, *models.EmailTranslation:
			// Drop only the redundant indexes explicitly selected below.
			_, err = engine.SyncWithOptions(xorm.SyncOptions{IgnoreDropIndices: true}, model)
		case *models.DMARCRecord:
			err = syncDMARCRecord(engine, model)
		case *models.MailboxAddress:
			err = syncAddressSchema(engine)
		default:
			err = engine.Sync2(model)
		}
		if err != nil {
			return fmt.Errorf("sync schema for %T: %w", model, err)
		}
	}
	if err := syncMailIndexes(engine); err != nil {
		return err
	}
	// The web/AI translation cache is already keyed with EmailID first.
	if err := dropRedundantModelIndex(engine, &models.EmailTranslation{}, "email_id"); err != nil {
		return err
	}
	return widenUserColumns(engine)
}

func WithContext(ctx *context.Context, sql string) string {
	if ctx != nil {
		logId := ctx.GetValue(context.LogID)
		return fmt.Sprintf("/* %s */ %s", logId, sql)
	}
	return sql
}

func fixHistoryData(engine *xorm.Engine, preserveUsers bool) error {
	var ueNum int
	_, err := engine.Table(&models.UserEmail{}).Select("count(1)").Get(&ueNum)
	if err != nil {
		return err
	}
	if ueNum > 0 {
		return nil
	}

	// 只有一个管理员用户
	var user []models.User
	err = engine.Table(&models.User{}).OrderBy("id asc").Find(&user)
	if err != nil {
		return err
	}

	// 只有一个账号，且不是管理员账号，将账号提权为管理员
	if !preserveUsers && len(user) == 1 && user[0].IsAdmin == 0 {
		u := user[0]
		u.IsAdmin = 1
		_, err = engine.ID(u.ID).Cols("is_admin").Update(&u)
		if err != nil {
			return err
		}
	}

	if len(user) != 1 {
		return nil
	}

	// 以前有邮件
	var emails []*models.Email
	err = engine.Table(&models.Email{}).Select("id,status").OrderBy("id asc").Find(&emails)
	if err != nil {
		return err
	}
	if len(emails) == 0 {
		return nil
	}

	log.Infof("Sync History Data！Please Wait！")

	// 把以前的邮件，全部分到管理员账号下面去
	for _, email := range emails {
		ue := models.UserEmail{
			UserID:  user[0].ID,
			EmailID: email.Id,
			Status:  email.Status,
		}
		_, err = engine.Insert(&ue)
		if err != nil {
			log.Errorf("SQL Error: %v", err)
		}
	}
	log.Infof("Sync History Data Finished. Num: %d", len(emails))
	return nil
}
