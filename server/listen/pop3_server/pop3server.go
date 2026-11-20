package pop3_server

import (
	"crypto/tls"
	"time"

	"github.com/Jinnrry/gopop"
	"github.com/Jinnrry/pmail/services/domain"
	log "github.com/sirupsen/logrus"
)

var instances []*gopop.Server

func StartWithTls(cert tls.Certificate) {
	start(995, true, cert)
}

func Start(cert tls.Certificate) {
	start(110, false, cert)
}

func start(port int, implicitTLS bool, cert tls.Certificate) {
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
	server := gopop.NewPop3Server(port, "pop."+domain.DefaultName(), implicitTLS, tlsConfig, action{})
	server.ConnectAliveTime = 5 * time.Minute
	instances = append(instances, server)
	go func() {
		if err := server.Start(); err != nil {
			log.Errorf("POP3 on port %d: %v", port, err)
		}
	}()
}

func Stop() {
	for _, server := range instances {
		server.Stop()
	}
	instances = nil
}
