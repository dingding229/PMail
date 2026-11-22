package setup

import (
	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/utils/array"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/errors"
)

var (
	ErrDbType = errors.New("invalid database type")
	ErrDbDSN  = errors.New("database DSN is required")
)

func GetDatabaseSettings(ctx *context.Context) (string, string, error) {
	configData, err := config.ReadConfig()
	if err != nil {
		return "", "", errors.Wrap(err)
	}

	if configData.DbType == "" {
		return config.DBTypePostgres, "", nil
	}

	return configData.DbType, configData.DbDSN, nil
}

func GetAdminAccount() (string, error) {
	if db.Instance == nil {
		return "", nil
	}
	var admin models.User
	_, err := db.Instance.Where("is_admin = 1").Asc("id").Get(&admin)
	return admin.Account, err
}

func SetDatabaseSettings(ctx *context.Context, dbType, dbDSN string) error {
	configData, err := config.ReadConfig()
	if err != nil {
		return errors.Wrap(err)
	}

	if !array.InArray(dbType, config.DBTypes) {
		return ErrDbType
	}

	if dbDSN == "" {
		return ErrDbDSN
	}

	configData.DbType = dbType
	configData.DbDSN = dbDSN

	err = config.WriteConfig(configData)
	if err != nil {
		return errors.Wrap(err)
	}
	if err := config.Init(); err != nil {
		return errors.Wrap(err)
	}
	// 检查数据库是否能正确连接
	err = db.Init("")
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}

// AutoInitialize marks the server configuration ready; user initialization is
// a separate database-backed operation after the domain is available.
func AutoInitialize() error {
	if _, err := auth.EnsureDKIM(config.Instance.DkimPrivateKeyPath); err != nil {
		return errors.Wrap(err)
	}
	if !config.IsInit {
		if err := markInitialized(); err != nil {
			return errors.Wrap(err)
		}
	}
	config.Instance.IsInit = true
	config.IsInit = true
	return nil
}
