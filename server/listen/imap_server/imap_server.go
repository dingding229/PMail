package imap_server

import (
	"crypto/tls"
	"net"
	"os"

	"github.com/Jinnrry/pmail/config"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	log "github.com/sirupsen/logrus"
)

var instanceTLS *imapserver.Server
var instanceSTARTTLS *imapserver.Server
var sockets []net.Listener

func Stop() {
	if instanceSTARTTLS != nil {
		instanceSTARTTLS.Close()
		instanceSTARTTLS = nil
	}
	if instanceTLS != nil {
		instanceTLS.Close()
		instanceTLS = nil
	}
	for _, socket := range sockets {
		socket.Close()
	}
	sockets = nil
}

// StarTLS listens for implicit TLS on port 993.
func StarTLS(cert tls.Certificate) {
	instanceTLS = start(":993", cert, true)
}

// StartSTARTTLS listens on port 143 but does not allow authentication before TLS.
func StartSTARTTLS(cert tls.Certificate) {
	instanceSTARTTLS = start(":143", cert, false)
}

func start(addr string, cert tls.Certificate, implicitTLS bool) *imapserver.Server {
	server := newIMAPServer(cert)
	socket, err := net.Listen("tcp", addr)
	if err != nil {
		log.Errorf("IMAP %s: %v", addr, err)
		return nil
	}
	if implicitTLS {
		socket = tls.NewListener(socket, &tls.Config{Certificates: []tls.Certificate{cert}})
	}
	sockets = append(sockets, socket)
	log.Infof("IMAP listening on %s (implicit TLS: %t)", addr, implicitTLS)
	go func() {
		if err := server.Serve(socket); err != nil {
			log.Warnf("IMAP %s: %v", addr, err)
		}
	}()
	return server
}

func newIMAPServer(cert tls.Certificate) *imapserver.Server {
	memServer := NewServer()
	option := &imapserver.Options{
		NewSession: func(conn *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return memServer.NewSession(), nil, nil
		},
		Caps: imap.CapSet{
			imap.CapIMAP4rev1: {},
			imap.CapIdle:      {},
		},
		TLSConfig:    &tls.Config{Certificates: []tls.Certificate{cert}},
		InsecureAuth: false,
	}
	if config.Instance.LogLevel == "debug" {
		option.DebugWriter = os.Stdout
	}
	return imapserver.New(option)
}
