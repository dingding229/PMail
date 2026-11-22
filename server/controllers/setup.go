package controllers

import (
	"errors"
	"net/http"
	"os"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/setup"
	"github.com/Jinnrry/pmail/services/setup/ssl"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

func AcmeChallenge(w http.ResponseWriter, r *http.Request) {
	log.Infof("AcmeChallenge: %s", r.URL.Path)
	instance := ssl.GetHttpChallengeInstance()
	if auth, exist := instance.Lookup(r.PathValue("token")); exist {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(auth))
	} else {
		http.Error(w, i18n.T(i18n.ForRequest(r), "not_found"), http.StatusNotFound)
	}
}

type sslResponse struct {
	Port int    `json:"port"`
	Type string `json:"type"`
}

func Setup(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	var data map[string]string
	if !readRequest(ctx, w, req, &data, false) {
		return
	}
	fail := func(key string, err error) {
		log.WithContext(ctx).Errorf("Legacy setup %s/%s failed: %v", data["step"], data["action"], err)
		response.Localized(ctx, response.ServerError, key).FPrint(w)
	}
	ack := func() { success(w, i18n.T(ctx, "succ")) }
	switch data["step"] + "/" + data["action"] {
	case "database/get":
		dbType, _, err := setup.GetDatabaseSettings(ctx)
		if err != nil {
			fail("setup_db_failed", err)
			return
		}
		success(w, map[string]string{"db_type": dbType})
	case "database/set":
		if err := setup.SetDatabaseSettings(ctx, data["db_type"], data["db_dsn"]); err == nil {
			ack()
		} else if errors.Is(err, setup.ErrDbType) {
			response.Localized(ctx, response.ParamsError, "setup_db_type").FPrint(w)
		} else if errors.Is(err, setup.ErrDbDSN) {
			response.Localized(ctx, response.ParamsError, "setup_db_dsn").FPrint(w)
		} else {
			fail("setup_db_failed", err)
		}
	case "password/get":
		account, err := setup.GetAdminAccount()
		if err != nil {
			fail("setup_db_failed", err)
			return
		}
		success(w, account)
	case "password/set", "account/set":
		apiError(ctx, w, http.StatusForbidden, response.NoAccessPrivileges, "setup_password_removed")
	case "domain/get":
		smtpDomain, webDomain, domains, err := setup.GetDomainSettings()
		if err != nil {
			fail("setup_domain_failed", err)
			return
		}
		success(w, map[string]any{"smtp_domain": smtpDomain, "web_domain": webDomain, "domains": domains})
	case "domain/set":
		if err := setup.SetDomainSettings(data["smtp_domain"], data["web_domain"], data["multi_domain"]); err == nil {
			ack()
		} else if errors.Is(err, setup.ErrDomainRequired) {
			response.Localized(ctx, response.ParamsError, "setup_domain_required").FPrint(w)
		} else if errors.Is(err, setup.ErrWebDomainRequired) {
			response.Localized(ctx, response.ParamsError, "setup_web_domain_required").FPrint(w)
		} else {
			fail("setup_domain_failed", err)
		}
	case "dns/get":
		records, err := setup.GetDNSSettings(ctx)
		if err != nil {
			fail("setup_dns_failed", err)
			return
		}
		success(w, records)
	case "ssl/get":
		success(w, sslResponse{Type: ssl.GetSSL(), Port: config.Instance.GetSetupPort()})
	case "ssl/getParams":
		success(w, ssl.GetDnsChallengeInstance().GetDNSSettings(ctx))
	case "ssl/set":
		sslType, keyPath, crtPath := data["ssl_type"], data["key_path"], data["crt_path"]
		if sslType == config.SSLTypeUser {
			for _, path := range []string{keyPath, crtPath} {
				if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
					response.Localized(ctx, response.ParamsError, "setup_ssl_file").FPrint(w)
					return
				}
			}
		}
		if err := ssl.SetSSL(sslType, keyPath, crtPath); err != nil {
			if errors.Is(err, ssl.ErrInvalidType) {
				response.Localized(ctx, response.ParamsError, "setup_ssl_type").FPrint(w)
			} else if errors.Is(err, ssl.ErrDomainRequired) {
				response.Localized(ctx, response.ParamsError, "setup_ssl_domain").FPrint(w)
			} else {
				fail("setup_ssl_failed", err)
			}
			return
		}
		// Automatic issuance runs after the normal listeners are started.
		if err := setup.Finish(); err != nil {
			fail("setup_ssl_failed", err)
			return
		}
		ack()
	default:
		response.Localized(ctx, response.ParamsError, "setup_invalid_step").FPrint(w)
	}
}
