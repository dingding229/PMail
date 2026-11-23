package send

import (
	stdcontext "context"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/queue"
	"github.com/Jinnrry/pmail/testschema"
	pcontext "github.com/Jinnrry/pmail/utils/context"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestIsPermanentSMTPResponse(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil"},
		{name: "network error", err: errors.New("connection refused")},
		{name: "temporary SMTP response", err: &textproto.Error{Code: 451, Msg: "try later"}},
		{name: "wrapped temporary SMTP response", err: fmt.Errorf("delivery: %w", &textproto.Error{Code: 421, Msg: "service unavailable"})},
		{name: "permanent SMTP response", err: &textproto.Error{Code: 550, Msg: "mailbox unavailable"}, want: true},
		{name: "wrapped permanent SMTP response", err: fmt.Errorf("delivery: %w", &textproto.Error{Code: 554, Msg: "transaction failed"}), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPermanentSMTPResponse(tt.err); got != tt.want {
				t.Fatalf("isPermanentSMTPResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeliveryFailureCausePreservesTemporaryMXError(t *testing.T) {
	tests := []struct {
		name      string
		lookupErr *net.DNSError
	}{
		{
			name:      "SERVFAIL before fallback NXDOMAIN",
			lookupErr: &net.DNSError{Err: "server misbehaving", Name: "example.com", IsTemporary: true},
		},
		{
			name:      "timeout before fallback NXDOMAIN",
			lookupErr: &net.DNSError{Err: "i/o timeout", Name: "example.com", IsTimeout: true},
		},
	}

	fallbackErr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: &net.DNSError{Err: "no such host", Name: "smtp.example.com", IsNotFound: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := deliveryFailureCause(tt.lookupErr, fallbackErr)

			var dnsErr *net.DNSError
			if !errors.As(err, &dnsErr) {
				t.Fatalf("deliveryFailureCause() = %T, want wrapped DNS error", err)
			}
			if dnsErr.IsNotFound {
				t.Fatalf("deliveryFailureCause() selected fallback NXDOMAIN: %v", dnsErr)
			}
			if !dnsErr.IsTemporary && !dnsErr.IsTimeout {
				t.Fatalf("deliveryFailureCause() selected non-temporary DNS error: %v", dnsErr)
			}
		})
	}
}

func TestDeliveryFailureCauseKeepsExplicitSMTPRejection(t *testing.T) {
	lookupErr := &net.DNSError{Err: "server misbehaving", Name: "example.com", IsTemporary: true}
	rejection := &textproto.Error{Code: 550, Msg: "mailbox unavailable"}

	if got := deliveryFailureCause(lookupErr, rejection); got != rejection {
		t.Fatalf("deliveryFailureCause() = %v, want explicit SMTP rejection %v", got, rejection)
	}
}

func TestSendMixedDeliveryKeepsOutboundQueueParentID(t *testing.T) {
	engine, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if err := testschema.Sync(engine); err != nil {
		t.Fatal(err)
	}
	oldEngine := db.Instance
	db.Instance = engine
	defer func() { db.Instance = oldEngine }()

	if err := domain.Initialize(&config.Config{Domain: "mixed-send.example"}); err != nil {
		t.Fatal(err)
	}
	user := &models.User{Account: "sender", Name: "Sender"}
	if _, err := engine.Insert(user); err != nil {
		t.Fatal(err)
	}
	parent := &models.Email{FromAddress: "sender@mixed-send.example", Status: consts.EmailStatusDeliveryPending}
	if _, err := engine.Insert(parent); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Insert(&models.UserEmail{UserID: user.ID, EmailID: parent.Id, Status: consts.EmailStatusDeliveryPending}); err != nil {
		t.Fatal(err)
	}

	oldDelivery := localDelivery
	defer func() { localDelivery = oldDelivery }()
	localDelivery = func(_ *pcontext.Context, email *parsemail.Email, _ []*parsemail.User) (map[string]error, error) {
		email.MessageId = 999999
		return map[string]error{}, nil
	}
	mail := &parsemail.Email{
		MessageId: int64(parent.Id),
		From:      &parsemail.User{EmailAddress: "sender@mixed-send.example"},
		To: []*parsemail.User{
			{EmailAddress: "local@mixed-send.example"},
			{EmailAddress: "remote@example.net"},
		},
		Subject: "mixed",
		Text:    []byte("body"),
	}
	ctx := &pcontext.Context{Context: stdcontext.Background(), UserID: user.ID}
	if results, _ := Send(ctx, mail); results["remote@example.net"] == nil {
		t.Fatal("external recipient was not queued")
	}
	var row models.OutboundQueue
	if ok, err := engine.Where("rcpt_to=?", "remote@example.net").Get(&row); err != nil || !ok {
		t.Fatalf("queue row lookup: ok=%v err=%v", ok, err)
	}
	if row.EmailID == nil || *row.EmailID != parent.Id {
		t.Fatalf("queue parent email_id = %v, want %d", row.EmailID, parent.Id)
	}
	if _, err := engine.ID(row.ID).Cols("status").Update(&models.OutboundQueue{Status: queue.StatusSent}); err != nil {
		t.Fatal(err)
	}
	if err := queue.ReconcileParent(parent.Id, user.ID); err != nil {
		t.Fatal(err)
	}
	var relation models.UserEmail
	if ok, err := engine.Where("email_id=? AND user_id=?", parent.Id, user.ID).Get(&relation); err != nil || !ok {
		t.Fatalf("parent relation lookup: ok=%v err=%v", ok, err)
	} else if relation.Status != consts.EmailStatusSent {
		t.Fatalf("parent relation status = %d, want %d", relation.Status, consts.EmailStatusSent)
	}
	var savedParent models.Email
	if ok, err := engine.ID(parent.Id).Get(&savedParent); err != nil || !ok {
		t.Fatalf("parent email lookup: ok=%v err=%v", ok, err)
	} else if savedParent.Status != consts.EmailStatusSent {
		t.Fatalf("parent email status = %d, want %d", savedParent.Status, consts.EmailStatusSent)
	}
}
