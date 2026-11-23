package res_init

import (
	stdcontext "context"
	"crypto/tls"
	"errors"
	"net/textproto"
	"slices"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/listen/cron_server"
	"github.com/Jinnrry/pmail/listen/http_server"
	"github.com/Jinnrry/pmail/listen/imap_server"
	"github.com/Jinnrry/pmail/listen/pop3_server"
	"github.com/Jinnrry/pmail/listen/smtp_server"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/compose"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/services/mailstore"
	"github.com/Jinnrry/pmail/services/queue"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/services/registration"
	"github.com/Jinnrry/pmail/services/setup"
	"github.com/Jinnrry/pmail/services/setup/ssl"
	"github.com/Jinnrry/pmail/services/telegram"
	"github.com/Jinnrry/pmail/services/update"
	"github.com/Jinnrry/pmail/services/user"
	"github.com/Jinnrry/pmail/session"
	"github.com/Jinnrry/pmail/signal"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/send"
	log "github.com/sirupsen/logrus"
)

func Init(serverVersion string) {
	_, _, bootstrap := config.BootstrapCredentials()
	if !config.IsInit && !bootstrap {
		if err := http_server.SetupStart(serverVersion); err != nil {
			log.Fatalf("Complete environment configuration or enable local legacy setup: %v", err)
		}
		<-signal.InitChan
		http_server.SetupStop()
		if err := config.Init(); err != nil {
			panic(err)
		}
	}

	for {
		if config.Instance.DbDSN == "" {
			log.Fatal("Database DSN must be configured; check config.json and PMAIL_* settings")
		}
		if err := db.InitStartup(serverVersion, true); err != nil {
			panic(err)
		}
		if err := ensureMailDomain(func() error {
			return http_server.DomainSetup(serverVersion, saveMailDomain)
		}); err != nil {
			panic(err)
		}
		if !config.IsInit {
			if err := setup.AutoInitialize(); err != nil {
				panic(err)
			}
		}
		if err := domain.Initialize(config.Instance); err != nil {
			panic(err)
		}
		if err := address.Backfill(); err != nil {
			panic(err)
		}
		account, password, _ := config.BootstrapCredentials()
		if err := registration.Prepare(stdcontext.Background(), account, password); err != nil {
			panic(err)
		}
		parsemail.Init()
		send.SetLocalDelivery(smtp_server.LocalDelivery)
		queue.SetExternalDelivery(func(deliveryCtx *context.Context, mailFrom string, raw []byte, recipient string) error {
			errorsByDomain, err := send.External(deliveryCtx, domain.DefaultName(), mailFrom, raw, []*parsemail.User{{EmailAddress: recipient}})
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
			_, localErr := smtp_server.LocalDelivery(deliveryCtx, email, []*parsemail.User{recipient})
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
			recipient, err := user.Get(row.UserID)
			if err != nil {
				log.Errorf("cannot resolve delivery notification user %d: %v", row.UserID, err)
				return
			}
			deliveryCtx.Lang = recipient.Lang
			key := "bounce_send_body"
			if row.Kind == queue.KindForward {
				key = "bounce_forward_body"
			}
			text := i18n.T(deliveryCtx, key, row.RcptTo, row.Attempts)
			if detail := i18n.Delivery(deliveryCtx, "delivery_detail", row.LastError); detail != "" {
				text += "\n\n" + detail
			}
			bounce := &parsemail.Email{
				From:    &parsemail.User{EmailAddress: "mailer-daemon@" + domain.DefaultName()},
				To:      []*parsemail.User{{EmailAddress: primary.Address}},
				Subject: i18n.T(deliveryCtx, "bounce_subject"),
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

		httpsReady := cert != nil && http_server.HttpsStart(*cert, serverVersion)
		// Bind HTTP before starting the scheduler so HTTP-01 can answer immediately.
		http_server.HttpStart(httpsReady, serverVersion)
		smtp_server.Start(cert)
		if cert != nil {
			startTLSListeners(*cert)
		}
		ctx, stopScheduler := stdcontext.WithCancel(stdcontext.Background())
		registration.StartNotices(ctx, cfg.WebDomain)
		inbound.SetNotifier(telegram.Notify)
		c9Workers := []<-chan struct{}{telegram.Start(ctx), compose.Start(ctx), update.Start(ctx, serverVersion)}
		quotaDone := make(chan struct{})
		go func() { defer close(quotaDone); quota.RunReconciliation(ctx) }()
		projectionDone := make(chan struct{})
		go func() {
			defer close(projectionDone)
			if err := mailstore.Backfill(ctx, db.Instance); err != nil && ctx.Err() == nil {
				log.Errorf("mail projection backfill: %v", err)
			}
		}()
		drainScheduler := cron_server.Start(ctx, cfg, cert)

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
		drainCtx, drainCancel := stdcontext.WithTimeout(stdcontext.Background(), 15*time.Second)
		drainErr := drainScheduler(drainCtx)
		if drainErr == nil {
			select {
			case <-projectionDone:
			case <-drainCtx.Done():
				drainErr = drainCtx.Err()
			}
		}
		if drainErr == nil {
			select {
			case <-quotaDone:
			case <-drainCtx.Done():
				drainErr = drainCtx.Err()
			}
		}
		for _, done := range c9Workers {
			if drainErr != nil {
				break
			}
			select {
			case <-done:
			case <-drainCtx.Done():
				drainErr = drainCtx.Err()
			}
		}

		drainCancel()
		if drainErr != nil {
			log.Errorf("scheduler did not drain; declining listener reload: %v", drainErr)
			return
		}
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

// ensureMailDomain lets PMail start without PMAIL_DOMAIN. An existing
// database keeps its default domain; a fresh install waits on the first-run
// page until an administrator with the registration key enters the domain.
func ensureMailDomain(interactive func() error) error {
	if config.Instance.Domain != "" {
		return nil
	}
	var row models.Domain
	found, err := db.Instance.Desc("is_default").Asc("id").Get(&row)
	if err != nil {
		return err
	}
	if !found {
		hasUsers, err := db.Instance.Exist(&models.User{})
		if err != nil {
			return err
		}
		if hasUsers {
			return errors.New("existing users but no mail domains: configure the original domain with PMAIL_DOMAIN before upgrading")
		}
		if err := interactive(); err != nil {
			return err
		}
		return config.Init()
	}
	cfg := config.Instance
	cfg.Domain = row.Name
	if !slices.Contains(cfg.Domains, row.Name) {
		cfg.Domains = append([]string{row.Name}, cfg.Domains...)
	}
	if cfg.WebDomain == "" {
		cfg.WebDomain = "mail." + row.Name
	}
	return nil
}

func saveMailDomain(mailDomain string) error {
	stored, err := config.ReadConfig()
	if err != nil {
		return err
	}
	stored.Domain = mailDomain
	// Fresh configurations start in proxy mode without a PMAIL_WEB_MODE
	// override. Keep that default once the persisted domain is no longer blank.
	stored.HttpsEnabled = 2
	return config.WriteConfig(stored)
}
