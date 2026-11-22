package res_init

import (
	stdcontext "context"
	"crypto/tls"
	"net/textproto"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/listen/cron_server"
	"github.com/Jinnrry/pmail/listen/http_server"
	"github.com/Jinnrry/pmail/listen/imap_server"
	"github.com/Jinnrry/pmail/listen/pop3_server"
	"github.com/Jinnrry/pmail/listen/smtp_server"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/queue"
	"github.com/Jinnrry/pmail/services/setup"
	"github.com/Jinnrry/pmail/services/setup/ssl"
	"github.com/Jinnrry/pmail/session"
	"github.com/Jinnrry/pmail/signal"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/send"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
)

func Init(serverVersion string) {
	account, password, bootstrap := config.BootstrapCredentials()
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
		queue.SetExternalDelivery(func(deliveryCtx *context.Context, mailFrom string, raw []byte, recipient string) error {
			err, errorsByDomain := send.External(deliveryCtx, domain.DefaultName(), mailFrom, raw, []*parsemail.User{{EmailAddress: recipient}})
			if err != nil {
				for _, deliveryErr := range errorsByDomain {
					if deliveryErr != nil {
						return deliveryErr
					}
				}
			}
			return err
		})
		queue.SetLocalDelivery(func(deliveryCtx *context.Context, email *parsemail.Email, recipient *parsemail.User) error {
			if recipient == nil {
				return address.ErrInvalid
			}
			localErr, _ := smtp_server.LocalDelivery(deliveryCtx, email, []*parsemail.User{recipient})
			return localErr
		})
		queue.SetBounceHandler(func(deliveryCtx *context.Context, row models.OutboundQueue) {
			if queue.ShouldSuppressBounce(row) {
				log.WithContext(deliveryCtx).Infof("suppress bounce notification queue=%d", row.ID)
				return
			}
			if row.Kind == queue.KindForward {
				reserved, reserveErr := queue.ReserveBounceNotice(row.UserID, row.RcptTo)
				if reserveErr != nil {
					log.WithContext(deliveryCtx).Errorf("reserve forwarding bounce notice user=%d target=%s failed: %v", row.UserID, row.RcptTo, reserveErr)
					return
				}
				if !reserved {
					log.WithContext(deliveryCtx).Infof("deduplicate forwarding bounce notice user=%d target=%s", row.UserID, row.RcptTo)
					return
				}
			}
			primary, err := address.Primary(row.UserID)
			if err != nil {
				log.Errorf("cannot create delivery failure notice for user %d: %v", row.UserID, err)
				return
			}
			text := "Your message to " + row.RcptTo + " could not be delivered after " + cast.ToString(row.Attempts) + " attempts.\n\n" + row.LastError
			if row.Kind == queue.KindForward {
				text = "转发到 " + row.RcptTo + " 失败。投递在 " + cast.ToString(row.Attempts) + " 次尝试后仍未成功。\n\n" + row.LastError
			}
			bounce := &parsemail.Email{
				From:    &parsemail.User{EmailAddress: "mailer-daemon@" + domain.DefaultName()},
				To:      []*parsemail.User{{EmailAddress: primary.Address}},
				Subject: "Delivery Status Notification (Failure)",
				Headers: textproto.MIMEHeader{"Auto-Submitted": {"auto-replied"}, "X-PMail-DSN": {"1"}},
				Text:    []byte(text),
			}
			_, _ = send.Send(deliveryCtx, bounce)
		})
		session.Init()
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
		ctx, stopScheduler := stdcontext.WithCancel(stdcontext.Background())
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
