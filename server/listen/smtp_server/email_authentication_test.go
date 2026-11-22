package smtp_server

import (
	"bytes"
	"net"
	"net/netip"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/testschema"
	"github.com/Jinnrry/pmail/utils/context"
	"xorm.io/xorm"
)

func TestReceivePersistsAuthenticationResult(t *testing.T) {
	engine := newAuthenticationTestEngine(t)

	oldDB := db.Instance
	oldConfig := config.Instance
	t.Cleanup(func() {
		db.Instance = oldDB
		config.Instance = oldConfig
	})

	db.Instance = engine
	config.Instance = &config.Config{
		Domain:          "test.domain",
		Domains:         []string{"test.domain"},
		SpamFilterLevel: 0,
		CatchAll:        true,
	}
	if err := domain.Initialize(config.Instance); err != nil {
		t.Fatalf("初始化测试域名失败：%v", err)
	}
	recipient := &models.User{Account: "recipient", Name: "收件人"}
	if _, err := engine.Insert(recipient); err != nil {
		t.Fatalf("创建测试收件人失败：%v", err)
	}
	if err := address.Backfill(); err != nil {
		t.Fatalf("初始化测试邮箱地址失败：%v", err)
	}

	emailData := []byte("From: sender\r\nTo: recipient@test.domain\r\nSubject: authentication flow\r\n\r\nbody")
	session := Session{
		RemoteAddress: net.TCPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.1:25")),
		Ctx:           &context.Context{},
		To:            []string{"recipient@test.domain"},
	}
	if err := session.Data(bytes.NewReader(emailData)); err != nil {
		t.Fatalf("收信流程执行失败：%v", err)
	}

	var saved models.Email
	has, err := engine.Where("subject = ?", "authentication flow").Get(&saved)
	if err != nil {
		t.Fatalf("查询落库邮件失败：%v", err)
	}
	if !has {
		t.Fatal("未找到落库邮件")
	}
	if saved.SPFCheck != 0 || saved.DKIMCheck != 0 {
		t.Fatalf("落库认证结果不正确：SPF=%d DKIM=%d", saved.SPFCheck, saved.DKIMCheck)
	}
}

func TestSaveSentEmailIgnoresReceiveAuthentication(t *testing.T) {
	engine := newAuthenticationTestEngine(t)

	oldDB := db.Instance
	t.Cleanup(func() { db.Instance = oldDB })
	db.Instance = engine

	email := &parsemail.Email{
		From:           &parsemail.User{EmailAddress: "sender@test.domain"},
		Subject:        "sent authentication isolation",
		Authentication: parsemail.NewEmailAuthentication(false, false),
	}
	_, saved, err := saveEmail(&context.Context{UserID: 1}, 0, email, 1, 1, nil, true, true)
	if err != nil {
		t.Fatalf("保存发件邮件失败：%v", err)
	}
	if saved.SPFCheck != 1 || saved.DKIMCheck != 1 {
		t.Fatalf("发件邮件认证落库值被收信认证对象覆盖：SPF=%d DKIM=%d", saved.SPFCheck, saved.DKIMCheck)
	}
}

func newAuthenticationTestEngine(t *testing.T) *xorm.Engine {
	t.Helper()
	engine, err := xorm.NewEngine("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("创建测试数据库失败：%v", err)
	}
	engine.SetMaxOpenConns(1)
	if err = testschema.Sync(engine); err != nil {
		t.Fatalf("初始化测试数据库失败：%v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	return engine
}
