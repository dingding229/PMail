package cron_server

import (
	"bytes"
	"context"
	"crypto/tls"
	"slices"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/queue"
	"github.com/Jinnrry/pmail/services/setup/ssl"
	"github.com/Jinnrry/pmail/signal"
	log "github.com/sirupsen/logrus"
)

const (
	certificateCheckInterval = 24 * time.Hour
	certificateRetryMin      = 10 * time.Minute
	certificateRetryMax      = 6 * time.Hour
)

// Start is called for every service generation, independently of the TLS mode.
// Future scheduler workers can share its context and stop on reload or shutdown.
func Start(ctx context.Context, cfg *config.Config, active *tls.Certificate) func(context.Context) error {
	go certificateMaintenance(ctx, cfg, active)
	go auth.RunCleanup(ctx)
	queueDone := queue.Start(ctx)
	return func(wait context.Context) error {
		select {
		case <-queueDone:
			return nil
		case <-wait.Done():
			return wait.Err()
		}
	}
}

func certificateMaintenance(ctx context.Context, cfg *config.Config, active *tls.Certificate) {
	if cfg.SSLType == config.SSLTypeDisabled {
		return
	}

	backoff := certificateRetryMin
	for ctx.Err() == nil {
		current := *cfg
		current.Domain, current.Domains = domain.DefaultName(), domain.Names()
		cert, err := ssl.LoadCertificate(&current)
		if err == nil && certificateChanged(active, cert) {
			requestCertificateReload(ctx)
			return
		}
		if ssl.Automatic(cfg) {
			var replaced bool
			replaced, err = ssl.MaintainCertificate(ctx, &current, cert)
			if replaced {
				cert, err = ssl.LoadCertificate(&current)
				if err == nil && certificateChanged(active, cert) {
					requestCertificateReload(ctx)
					return
				}
			}
		}
		if ctx.Err() != nil {
			return
		}

		delay := certificateCheckInterval
		if err != nil {
			delay = backoff
			backoff = min(2*backoff, certificateRetryMax)
			if active == nil {
				log.Errorf("TLS certificate unavailable; retrying in %s: %v", delay, err)
			} else {
				log.Warnf("TLS certificate maintenance failed; retaining the active certificate and retrying in %s: %v", delay, err)
			}
		} else {
			backoff = certificateRetryMin
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func certificateChanged(active, loaded *tls.Certificate) bool {
	return active == nil || !slices.EqualFunc(active.Certificate, loaded.Certificate, bytes.Equal)
}

func requestCertificateReload(ctx context.Context) {
	log.Info("TLS certificate is ready or replaced; reloading listeners")
	select {
	case signal.RestartChan <- true:
	case <-ctx.Done():
	}
}
