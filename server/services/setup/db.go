package setup

import (
	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/utils/array"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/errors"
	"github.com/Jinnrry/pmail/utils/password"
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

func GetAdminPassword(ctx *context.Context) (string, error) {

	users := []*models.User{}
	err := db.Instance.Find(&users)
	if err != nil {
		return "", errors.Wrap(err)
	}

	if len(users) > 0 {
		return users[0].Account, nil
	}

	return "", nil
}

func SetAdminPassword(ctx *context.Context, account, pwd string) error {
	encodePwd, err := password.Hash(pwd)
	if err != nil {
		return err
	}
	var user models.User = models.User{
		Account:  account,
		Name:     "admin",
		Password: encodePwd,
		IsAdmin:  1,
	}

	_, err = db.Instance.Insert(&user)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func SetDatabaseSettings(ctx *context.Context, dbType, dbDSN string) error {
	configData, err := config.ReadConfig()
	if err != nil {
		return errors.Wrap(err)
	}

	if !array.InArray(dbType, config.DBTypes) {
		return errors.New("dbtype error")
	}

	if dbDSN == "" {
		return errors.New("DSN error")
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

// AutoInitialize creates the configured admin if absent and marks this config as initialized.
// The caller has already opened and initialized the database.
func AutoInitialize(account, rawPassword string) error {
	var existing models.User
	found, err := db.Instance.Where("account = ?", account).Get(&existing)
	if err != nil {
		return errors.Wrap(err)
	}
	if !found {
		hash, err := password.Hash(rawPassword)
		if err != nil {
			return err
		}
		_, err = db.Instance.Insert(&models.User{
			Account:  account,
			Name:     "admin",
			Password: hash,
			IsAdmin:  1,
		})
		if err != nil {
			return errors.Wrap(err)
		}
	}
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
