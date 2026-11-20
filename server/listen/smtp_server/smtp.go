package smtp_server

import (
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/emersion/go-smtp"
	log "github.com/sirupsen/logrus"
)

type listener struct {
	server *smtp.Server
	socket net.Listener
}

var listeners []listener

func newSMTPServer(addr string) *smtp.Server {
	server := smtp.NewServer(&Backend{})
	server.Addr = addr
	server.Domain = config.Instance.Domain
	server.ReadTimeout = 10 * time.Second
	server.WriteTimeout = 10 * time.Second
	server.MaxMessageBytes = 1024 * 1024 * 30
	server.MaxRecipients = 50
	server.AllowInsecureAuth = false
	return server
}

func Start(cert *tls.Certificate) {
	start(":25", cert, false)
}

func StartWithTLS(cert tls.Certificate) {
	start(":465", &cert, true)
}

func StartWithTLSNew(cert tls.Certificate) {
	start(":587", &cert, false)
}

func start(addr string, cert *tls.Certificate, implicitTLS bool) {
	server := newSMTPServer(addr)
	if cert != nil {
		server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{*cert}}
	}
	socket, err := net.Listen("tcp", addr)
	if err != nil {
		if addr == ":25" {
			log.Fatalf("SMTP %s: %v", addr, err)
		}
		log.Errorf("SMTP %s: %v", addr, err)
		return
	}
	if implicitTLS {
		socket = tls.NewListener(socket, server.TLSConfig)
	}
	listeners = append(listeners, listener{server, socket})
	log.Infof("SMTP listening on %s (implicit TLS: %t, certificate loaded: %t)", addr, implicitTLS, cert != nil)
	go func() {
		if err := server.Serve(socket); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Errorf("SMTP %s: %v", addr, err)
		}
	}()
}

func Stop() {
	for _, l := range listeners {
		l.server.Close()
		// Also close the socket if Serve had not registered it yet.
		l.socket.Close()
	}
	listeners = nil
}
