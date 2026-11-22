package external

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/utils/mailbudget"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

type oversizedLiteral struct{ t *testing.T }

func (l oversizedLiteral) Read([]byte) (int, error) {
	l.t.Fatal("oversized literal was read")
	return 0, nil
}
func (oversizedLiteral) Size() int64 { return mailbudget.Message + 1 }

func TestC7bExternalLiteralAndPOP3Budgets(t *testing.T) {
	if _, err := readExternalLiteral(oversizedLiteral{t}); !errors.Is(err, mailbudget.ErrLimit) {
		t.Fatal(err)
	}
	if _, err := readPOP3(bufio.NewReader(strings.NewReader("+OK " + strings.Repeat("x", mailbudget.POPLine) + "\r\n"))); !errors.Is(err, mailbudget.ErrLimit) {
		t.Fatal(err)
	}
	listing := "+OK\r\n" + strings.Repeat(strings.Repeat("x", 4096)+"\r\n", mailbudget.POPListing/4096+2) + ".\r\n"
	if _, err := readMultiPOP3(bufio.NewReader(strings.NewReader(listing))); !errors.Is(err, mailbudget.ErrLimit) {
		t.Fatal(err)
	}
}

func TestC7bExternalSchedulerRecoversAndDrains(t *testing.T) {
	started := make(chan int, 3)
	s := newScheduler(context.Background(), 1, func(ctx context.Context, id int) error {
		started <- id
		if id == 1 {
			panic("fixture")
		}
		<-ctx.Done()
		return ctx.Err()
	})
	if err := s.launch(1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("not started")
	}
	deadline := time.Now().Add(time.Second)
	for s.launch(2) != nil {
		if time.Now().After(deadline) {
			t.Fatal("panic leaked slot")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("second not started")
	}
	if s.launch(2) == nil || s.launch(3) == nil {
		t.Fatal("admission failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.drain(ctx); err != nil || len(s.active) != 0 || len(s.permits) != 0 || s.launch(4) == nil {
		t.Fatalf("not drained: %v", err)
	}
}

func TestC7bExternalImportAndSeenAreAtomic(t *testing.T) {
	e := newExternalTestEngine(t)
	old := config.Instance
	config.Instance = &config.Config{Domain: "example.test", Domains: []string{"example.test"}}
	t.Cleanup(func() { config.Instance = old })
	if err := domain.Initialize(config.Instance); err != nil {
		t.Fatal(err)
	}
	u := models.User{Account: "reader", Password: "hash"}
	if _, err := e.Insert(&u); err != nil {
		t.Fatal(err)
	}
	if err := address.Backfill(); err != nil {
		t.Fatal(err)
	}
	a := models.ExternalAccount{UserID: u.ID, Name: "fixture", PasswordEnc: "fixture"}
	if _, err := e.Insert(&a); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	raw := []byte("From: sender@example.net\r\nMessage-ID: <atomic@example.net>\r\nSubject: atomic\r\n\r\nbody")
	if _, err := e.Exec("CREATE TRIGGER fixture_abort BEFORE INSERT ON email BEGIN SELECT RAISE(ABORT, 'fixture rollback'); END"); err != nil {
		t.Fatal(err)
	}
	if err := importMessage(ctx, a, "INBOX", 2, 4, "", raw); err == nil || seen(a.ID, "INBOX", 2, 4, "") {
		t.Fatalf("mail/seen partial commit: %v", err)
	}
	if _, err := e.Exec("DROP TRIGGER fixture_abort"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := importMessage(ctx, a, "INBOX", 2, 4, "", raw); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := e.Count(&models.Email{}); err != nil || n != 1 || !seen(a.ID, "INBOX", 2, 4, "") {
		t.Fatalf("dedup count=%d err=%v", n, err)
	}
}

func TestC7bExternalDeleteUsesOnlyImportedUIDs(t *testing.T) {
	ids := imap.UIDSetNum(3, 9)
	for _, capable := range []bool{false, true} {
		marks, purges := 0, 0
		if err := deleteImported(ids, capable, func(got imap.UIDSet) error {
			marks++
			if got.String() != "3,9" {
				t.Fatal(got)
			}
			return nil
		}, func(got imap.UIDSet) error {
			purges++
			if got.String() != "3,9" {
				t.Fatal(got)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if marks != 1 || (purges == 1) != capable {
			t.Fatal(marks, purges)
		}
	}
}

func TestC7bIMAPAbortsOversizedLiteralWithoutDrainingNetwork(t *testing.T) {
	remote, local := net.Pipe()
	defer remote.Close()
	defer local.Close()
	go func() {
		defer remote.Close()
		if _, err := fmt.Fprint(remote, "* OK [CAPABILITY IMAP4rev1 UIDPLUS] fixture\r\n"); err != nil {
			return
		}
		line, err := bufio.NewReader(remote).ReadString('\n')
		if err != nil || !strings.Contains(line, "FETCH") {
			return
		}
		// No literal bytes or closing parenthesis ever arrive from this peer.
		if _, err = fmt.Fprintf(remote, "* 1 FETCH (UID 4 BODY[] {%d}\r\n", mailbudget.Message+1); err != nil {
			return
		}
		_, _ = remote.Read(make([]byte, 1))
	}()
	c := &externalIMAP{Client: imapclient.New(local, nil), abort: func() { _ = local.Close() }}
	finished := make(chan error, 1)
	go func() {
		cmd := c.Fetch(imap.UIDSetNum(4), &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{{Peek: true}}})
		msg := cmd.Next()
		if msg == nil {
			finished <- errors.New("fixture provided no message")
			return
		}
		_, _, err := readIMAPMail(msg)
		abortFetch(c, cmd)
		_ = c.Close()
		finished <- err
	}()
	select {
	case err := <-finished:
		if !errors.Is(err, mailbudget.ErrLimit) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("oversized response blocked on remote literal")
	}
}
