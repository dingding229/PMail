package res_init

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/hooks"
	"github.com/Jinnrry/pmail/listen/cron_server"
	"github.com/Jinnrry/pmail/listen/http_server"
	"github.com/Jinnrry/pmail/listen/imap_server"
	"github.com/Jinnrry/pmail/listen/pop3_server"
	"github.com/Jinnrry/pmail/listen/smtp_server"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/setup"
	"github.com/Jinnrry/pmail/services/setup/ssl"
	"github.com/Jinnrry/pmail/session"
	"github.com/Jinnrry/pmail/signal"
	"github.com/Jinnrry/pmail/utils/send"
	log "github.com/sirupsen/logrus"
)

func Init(serverVersion string) {
	account, password, bootstrap := config.BootstrapCredentials()
	if !config.IsInit && bootstrap && password == "" {
		log.Fatal("PMAIL_ADMIN_PASSWORD is required for first-time headless initialization; set it before starting PMail")
	}
	if !config.IsInit && !bootstrap {
		go http_server.SetupStart()
		<-signal.InitChan
		http_server.SetupStop()
		if err := config.Init(); err != nil {
			panic(err)
		}
	}

	preserveUsers := bootstrap
	for {
		if config.Instance.DbDSN == "" {
			log.Fatal("Database DSN must be configured; check config.json and PMAIL_* settings")
		}
		if err := db.InitStartup(serverVersion, preserveUsers); err != nil {
			panic(err)
		}
		if bootstrap && !config.IsInit {
			if err := setup.AutoInitialize(account, password); err != nil {
				panic(err)
			}
		}
		if err := domain.Initialize(config.Instance); err != nil {
			panic(err)
		}
		if err := address.Backfill(); err != nil {
			panic(err)
		}
		parsemail.Init()
		send.SetLocalDelivery(smtp_server.LocalDelivery)
		session.Init()
		hooks.Init(serverVersion)
		cfg := config.Instance
		var cert *tls.Certificate
		if cfg.SSLType != config.SSLTypeDisabled {
			var err error
			cert, err = ssl.LoadCertificate(cfg)
			if err != nil {
				log.Errorf("TLS certificate unavailable; serving HTTP and SMTP 25 without TLS: %v", err)
			}
		}

		httpsReady := cert != nil && http_server.HttpsStart(*cert)
		// Bind HTTP before starting the scheduler so HTTP-01 can answer immediately.
		http_server.HttpStart(httpsReady)
		smtp_server.Start(cert)
		if cert != nil {
			startTLSListeners(*cert)
		}
		ctx, stopScheduler := context.WithCancel(context.Background())
		cron_server.Start(ctx, cfg, cert)

		restart := false
		select {
		case <-signal.RestartChan:
			restart = true
			log.Info("Server restarting")
		case <-signal.StopChan:
			log.Info("Server stopping")
		}
		stopScheduler()
		smtp_server.Stop()
		http_server.HttpsStop()
		http_server.HttpStop()
		pop3_server.Stop()
		imap_server.Stop()
		hooks.Stop()
		session.Stop()
		if !restart {
			db.Instance.Close()
			return
		}
		time.Sleep(5 * time.Second)
		if err := config.Init(); err != nil {
			panic(err)
		}
	}
}

func startTLSListeners(cert tls.Certificate) {
	smtp_server.StartWithTLS(cert)
	smtp_server.StartWithTLSNew(cert)
	pop3_server.Start(cert)
	pop3_server.StartWithTls(cert)
	imap_server.StarTLS(cert)
	imap_server.StartSTARTTLS(cert)
}
