package setup

import (
	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/signal"
	"github.com/Jinnrry/pmail/utils/errors"
)

// Finish 标记初始化完成
func Finish() error {
	if err := markInitialized(); err != nil {
		return err
	}
	signal.InitChan <- true
	return nil
}

func markInitialized() error {
	cfg, err := config.ReadConfig()
	if err != nil {
		return errors.Wrap(err)
	}
	cfg.IsInit = true

	err = config.WriteConfig(cfg)
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}
