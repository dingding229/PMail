package pop3_server

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/maintenance"
	log "github.com/sirupsen/logrus"
)

type popServer struct {
	listener net.Listener
	mu       sync.Mutex
	conns    map[net.Conn]struct{}
	closing  bool
	wg       sync.WaitGroup
}

var instances []*popServer // listener lifecycle is owned by res_init

func StartWithTls(cert tls.Certificate) { start(995, true, cert) }
func Start(cert tls.Certificate)        { start(110, false, cert) }

func start(port int, implicitTLS bool, cert tls.Certificate) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Errorf("POP3 on port %d: %v", port, err)
		return
	}
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
	var implicit *tls.Config
	if implicitTLS {
		implicit = tlsConfig
	}
	listener = maintenance.Listen(listener, implicit, "-ERR [SYS/TEMP] maintenance\r\n")
	s := &popServer{listener: listener, conns: make(map[net.Conn]struct{})}
	instances = append(instances, s)
	host := "pop." + domain.DefaultName()
	s.wg.Go(func() { s.serve(host, implicitTLS, tlsConfig) })
}

func (s *popServer) serve(host string, implicitTLS bool, tlsConfig *tls.Config) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				log.Errorf("POP3 accept: %v", err)
			}
			return
		}
		s.mu.Lock()
		if s.closing {
			s.mu.Unlock()
			conn.Close()
			return
		}
		release, ok := admission.reserve(socketSource(conn.RemoteAddr()), envBound("PMAIL_POP3_MAX_CONNECTIONS", 128, 4096), envBound("PMAIL_POP3_MAX_PER_IP", 8, 4096))
		if !ok {
			s.mu.Unlock()
			_ = conn.Close()
			continue
		}
		s.conns[conn] = struct{}{}
		s.wg.Go(func() {
			defer release()
			defer func() { conn.Close(); s.mu.Lock(); delete(s.conns, conn); s.mu.Unlock() }()
			serveConnection(conn, host, implicitTLS, tlsConfig)
		})
		s.mu.Unlock()
	}
}

func Stop() {
	for _, s := range instances {
		s.mu.Lock()
		s.closing = true
		s.listener.Close()
		for conn := range s.conns {
			conn.Close()
		}
		s.mu.Unlock()
		s.wg.Wait()
	}
	instances = nil
}

func serveConnection(conn net.Conn, host string, secure bool, tlsConfig *tls.Config) {
	idleTimeout := popIdleTimeout()
	reader, writer := bufio.NewReaderSize(conn, 4096), bufio.NewWriter(conn)
	send := func(response string) error {
		if _, err := writer.WriteString(response + "\r\n"); err != nil {
			return err
		}
		return writer.Flush()
	}
	multi := func(status, body string) error {
		if _, err := writer.WriteString(status + "\r\n"); err != nil {
			return err
		}
		dot := textproto.NewWriter(writer).DotWriter()
		if _, err := io.WriteString(dot, body); err != nil {
			return err
		}
		return dot.Close() // dot-stuff and flush the final line
	}
	allowPlainAuth := config.Flag("PMAIL_POP3_ALLOW_PLAINTEXT_AUTH")
	var login string
	var drop *maildrop // nil is AUTHORIZATION; otherwise TRANSACTION until QUIT
	defer func() {
		if drop != nil {
			drop.close()
		}
	}() // EOF/timeouts never commit DELE
	_ = conn.SetDeadline(time.Now().Add(idleTimeout))
	if err := send("+OK " + host + " POP3 ready"); err != nil {
		return
	}
	for {
		_ = conn.SetDeadline(time.Now().Add(idleTimeout))
		raw, err := reader.ReadSlice('\n')
		if err != nil {
			return
		}
		line := strings.TrimRight(string(raw), "\r\n")
		command, arg, _ := strings.Cut(line, " ")
		command = strings.ToUpper(command)
		switch command {
		case "CAPA":
			capabilities := "TOP\r\nUIDL\r\n"
			if secure || allowPlainAuth {
				capabilities += "USER\r\n"
			}
			if drop == nil && !secure && tlsConfig != nil {
				capabilities += "STLS\r\n"
			}
			err = multi("+OK Capability list follows", capabilities)
		case "QUIT":
			if drop != nil {
				// UPDATE: apply marks only for an authenticated QUIT.
				if err = drop.validate(); err == nil {
					err = drop.commit()
				}
				if err != nil {
					log.WithContext(drop.ctx).Warnf("POP3 UPDATE: %v", err)
					_ = send("-ERR could not update maildrop")
					return
				}
			}
			_ = send("+OK bye")
			return
		case "USER":
			if !secure && !allowPlainAuth {
				err = send("-ERR STLS is required before authentication")
				break
			}
			if drop != nil {
				err = send("-ERR already signed in")
				break
			}
			fields := strings.Fields(arg)
			if len(fields) != 1 {
				login = ""
				err = send("-ERR username required")
				break
			}
			login = fields[0]
			err = send("+OK")
		case "PASS":
			if !secure && !allowPlainAuth {
				err = send("-ERR STLS is required before authentication")
				break
			}
			if drop != nil {
				err = send("-ERR already signed in")
				break
			}
			if login == "" || arg == "" {
				err = send("-ERR USER and PASS required")
				break
			}
			user, appID, authErr := auth.AuthenticateMail(login, arg, conn.RemoteAddr().String())
			if authErr != nil {
				err = send("-ERR authentication failed")
				break
			}
			drop, err = openMaildrop(user, appID)
			if err != nil {
				if errors.Is(err, errMaildropLocked) {
					err = send("-ERR " + errMaildropLocked.Error())
				} else {
					log.Errorf("POP3 snapshot: %v", err)
					err = send("-ERR mailbox unavailable")
				}
				break
			}
			err = send("+OK User signed in")
		case "STLS":
			if drop != nil || secure || tlsConfig == nil {
				err = send("-ERR STLS not available")
				break
			}
			// Do not accept buffered plaintext as input within the TLS session.
			if reader.Buffered() != 0 {
				_ = send("-ERR STLS must not be pipelined")
				return
			}
			if err = send("+OK Begin TLS negotiation"); err != nil {
				return
			}
			upgraded := tls.Server(conn, tlsConfig)
			if err = upgraded.Handshake(); err != nil {
				return
			}
			conn = upgraded
			reader, writer = bufio.NewReaderSize(conn, 4096), bufio.NewWriter(conn)
			secure, login = true, ""
		default:
			if drop == nil || drop.validate() != nil {
				err = send("-ERR sign in first")
				break
			}
			err = transactionCommand(drop, command, strings.Fields(arg), send, multi)
		}
		if err != nil {
			return
		}
	}
}

func transactionCommand(d *maildrop, command string, args []string, send func(string) error, multi func(string, string) error) error {
	failure := func(err error) error {
		if errors.Is(err, errNoSuchMessage) {
			return send("-ERR no such message")
		}
		log.WithContext(d.ctx).Errorf("POP3 %s: %v", command, err)
		return send("-ERR mailbox unavailable")
	}
	switch command {
	case "STAT":
		n, total, err := d.stat()
		if err != nil {
			return failure(err)
		}
		return send(fmt.Sprintf("+OK %d %d", n, total))
	case "LIST", "UIDL":
		var number int
		if len(args) > 0 {
			var err error
			number, err = strconv.Atoi(args[0])
			if len(args) != 1 || err != nil || number <= 0 {
				return failure(errNoSuchMessage)
			}
		}
		lines, err := d.list(number, command == "UIDL")
		if err != nil {
			return failure(err)
		}
		if number > 0 {
			return send("+OK " + lines[0])
		}
		return multi("+OK", strings.Join(lines, "\r\n"))
	case "RETR", "TOP", "DELE":
		if len(args) == 0 {
			return failure(errNoSuchMessage)
		}
		number, err := strconv.Atoi(args[0])
		if err != nil || number <= 0 {
			return failure(errNoSuchMessage)
		}
		if command == "DELE" {
			if len(args) != 1 {
				return send("-ERR invalid arguments")
			}
			if err := d.mark(number); err != nil {
				return failure(err)
			}
			return send("+OK marked")
		}
		var top int
		if command == "TOP" {
			if len(args) != 2 {
				return send("-ERR invalid arguments")
			}
			top, err = strconv.Atoi(args[1])
			if err != nil || top < 0 {
				return send("-ERR invalid line count")
			}
		} else if len(args) != 1 {
			return send("-ERR invalid arguments")
		}
		body, err := d.retr(number)
		if err != nil {
			return failure(err)
		}
		if command == "TOP" {
			return multi("+OK", topLines(body, top))
		}
		return multi(fmt.Sprintf("+OK %d octets", len(body)), body)
	case "RSET", "REST": // REST was accepted by the old transport; keep it as an alias.
		d.reset()
		return send("+OK")
	case "NOOP":
		return send("+OK")
	default:
		return send("-ERR unsupported command")
	}
}
