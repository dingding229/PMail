package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/migration"
	"github.com/Jinnrry/pmail/res_init"
	log "github.com/sirupsen/logrus"
)

var (
	gitHash   string
	buildTime string
	goVersion string
	version   string
)

func main() {

	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := migration.Run(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := healthcheck(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := config.Init(); err != nil {
		log.Fatal(err)
	}

	if version == "" {
		version = "TestVersion"
	}

	log.Infoln("*******************************************************************")
	log.Infof("***\tServer Start Success \n")
	log.Infof("***\tServer Version: %s \n", version)
	log.Infof("***\tGit Commit Hash: %s ", gitHash)
	log.Infof("***\tBuild Date: %s ", buildTime)
	log.Infof("***\tBuild GoLang Version: %s ", goVersion)
	log.Infoln("*******************************************************************")

	// 核心服务启动
	res_init.Init(version)

	log.Warnf("Server Stoped \n")

}

// healthcheck reads the same HTTP port as the server without initializing config or the database.
func healthcheck() error {
	cfg, err := config.ReadEffectiveConfig()
	if err != nil {
		return err
	}
	port := cfg.HttpPort
	if port == 0 {
		port = 80
	}
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/ping", port))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned HTTP %d", resp.StatusCode)
	}
	return nil
}
