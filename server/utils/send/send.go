package send

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/utils/array"
	"github.com/Jinnrry/pmail/utils/async"
	uconsts "github.com/Jinnrry/pmail/utils/consts"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/smtp"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

type LocalDeliveryFunc func(*context.Context, *parsemail.Email, []*parsemail.User) (error, map[string]error)

var localDelivery LocalDeliveryFunc

func SetLocalDelivery(fn LocalDeliveryFunc) { localDelivery = fn }

type mxDomain struct {
	domain string
	mxHost string
}

// mxFailoverEntry 记录一个收件人域名的全部 MX 主机（按优先级排序）。
type mxFailoverEntry struct {
	domain  string
	mxHosts []string
}

type temporaryMXFallbackError struct {
	lookupErr   error
	fallbackErr error
}

func (e *temporaryMXFallbackError) Error() string {
	return fmt.Sprintf("temporary MX lookup failed (%v); fallback delivery failed: %v", e.lookupErr, e.fallbackErr)
}

func (e *temporaryMXFallbackError) Unwrap() error {
	return e.lookupErr
}

// Forward 转发邮件
func Forward(ctx *context.Context, e *parsemail.Email, forwardAddress string, user *models.User) error {

	log.WithContext(ctx).Debugf("开始转发邮件")

	primary, err := address.Primary(user.ID)
	if err != nil {
		return err
	}
	b := e.ForwardBuildBytes(ctx, user, forwardAddress, primary.Address)

	log.WithContext(ctx).Debugf("%s", b)

	return forwardData(ctx, primary.Domain, b, forwardAddress, primary.Address)
}

func ForwardRaw(ctx *context.Context, e *parsemail.Email, rawEmailData []byte, forwardAddress string, user *models.User) error {
	log.WithContext(ctx).Debugf("开始原始邮件转发")

	primary, err := address.Primary(user.ID)
	if err != nil {
		return err
	}
	return forwardData(ctx, primary.Domain, rawEmailData, forwardAddress, primary.Address)
}

func forwardData(ctx *context.Context, fromDomain string, data []byte, forwardAddress string, from string) error {
	var to []*parsemail.User
	to = []*parsemail.User{
		{EmailAddress: forwardAddress},
	}

	err, _ := doSend(ctx, fromDomain, data, to, from)
	return err
}

func Send(ctx *context.Context, e *parsemail.Email) (error, map[string]error) {
	_, fromDomain := e.From.GetDomainAccount()
	// The signed bytes are the actual message delivered to SMTP and are also
	// the authoritative size for local quota accounting and persisted records.
	data := e.BuildBytes(ctx, true)
	e.Size = len(data)
	all := append(append(append([]*parsemail.User{}, e.To...), e.Cc...), e.Bcc...)
	var local, external []*parsemail.User
	for _, recipient := range all {
		_, host, ok := strings.Cut(recipient.EmailAddress, "@")
		if ok && domain.IsLocal(host) {
			local = append(local, recipient)
		} else {
			external = append(external, recipient)
		}
	}
	var first error
	deliveryErrors := make(map[string]error)
	if len(local) > 0 {
		var err error
		var localErrors map[string]error
		if localDelivery != nil {
			err, localErrors = localDelivery(ctx, e, local)
		} else {
			err = deliverLocal(ctx, e, local)
		}
		for address, deliveryErr := range localErrors {
			deliveryErrors[address] = deliveryErr
		}
		if err != nil {
			first = err
		}
	}
	if len(external) > 0 {
		err, externalErrors := doSend(ctx, fromDomain, data, external, e.From.EmailAddress)
		for address, deliveryErr := range externalErrors {
			deliveryErrors[address] = deliveryErr
		}
		if err != nil && first == nil {
			first = err
		}
	}
	return first, deliveryErrors
}

func deliverLocal(ctx *context.Context, e *parsemail.Email, recipients []*parsemail.User) error {
	ids := []int{}
	seen := map[int]bool{}
	for _, r := range recipients {
		u, found, err := address.Resolve(r.EmailAddress)
		if err != nil {
			return err
		}
		if found && !seen[u.ID] {
			ids = append(ids, u.ID)
			seen[u.ID] = true
		}
	}
	if len(ids) == 0 {
		return address.ErrNotFound
	}
	model := models.Email{Type: int8(consts.EmailTypeReceive), Subject: e.Subject, FromName: e.From.Name, FromAddress: e.From.EmailAddress, To: jsonString(recipients), Text: sql.NullString{String: string(e.Text), Valid: true}, Html: sql.NullString{String: string(e.HTML), Valid: true}, Sender: jsonString(e.Sender), Attachments: jsonString(e.Attachments), Size: e.Size, SPFCheck: 1, DKIMCheck: 1, Status: 0, SendDate: time.Now(), CreateTime: time.Now(), CronSendTime: time.Now(), MsgID: e.MsgID}
	if model.MsgID == "" {
		model.MsgID = fmt.Sprintf("<local-%d@%s>", time.Now().UnixNano(), domain.DefaultName())
	}
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		for _, id := range ids {
			if err := quota.LockAndCheck(tx, id, int64(model.Size)); err != nil {
				return err
			}
		}
		if _, err := tx.Insert(&model); err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := tx.Insert(&models.UserEmail{UserID: id, EmailID: model.Id, Status: 0}); err != nil {
				return err
			}
		}
		return nil
	})
}

func jsonString(value any) string { b, _ := json.Marshal(value); return string(b) }

func External(ctx *context.Context, fromDomain, envelope string, raw []byte, recipients []*parsemail.User) (error, map[string]error) {
	return doSend(ctx, fromDomain, raw, recipients, envelope)
}

func doSend(ctx *context.Context, fromDomain string, data []byte, to []*parsemail.User, from string) (error, map[string]error) {

	// 按域名整理
	toByDomain := map[mxDomain][]*parsemail.User{}
	mxLookupErrors := map[mxDomain]error{}
	// mxFailoverMap 保存每个域名的完整 MX 列表（按优先级排序），用于故障转移
	mxFailoverMap := map[string][]string{}
	for _, s := range to {
		args := strings.Split(s.EmailAddress, "@")
		if len(args) == 2 {
			if args[1] == uconsts.TEST_DOMAIN {
				// 测试使用
				address := mxDomain{
					domain: "localhost",
					mxHost: "127.0.0.1",
				}
				toByDomain[address] = append(toByDomain[address], s)
			} else {
				//查询dns mx记录
				mxInfo, lookupErr := net.LookupMX(args[1])
				address := mxDomain{
					domain: "smtp." + args[1],
					mxHost: "smtp." + args[1],
				}
				if lookupErr != nil {
					log.WithContext(ctx).Errorf("%s 域名mx记录查询失败，检查邮箱是否存在！", s.EmailAddress)
				}
				if len(mxInfo) > 0 {
					address = mxDomain{
						domain: args[1],
						mxHost: mxInfo[0].Host,
					}
					// 保存全部 MX 主机（net.LookupMX 已按优先级排序）
					allHosts := make([]string, 0, len(mxInfo))
					for _, mx := range mxInfo {
						allHosts = append(allHosts, mx.Host)
					}
					mxFailoverMap[args[1]] = allHosts
				}
				if lookupErr != nil {
					mxLookupErrors[address] = lookupErr
				}
				toByDomain[address] = append(toByDomain[address], s)
			}
		} else {
			log.WithContext(ctx).Errorf("邮箱地址解析错误！ %s", s)
			continue
		}
	}

	var errEmailAddress []string
	var errEmailAddressMu sync.Mutex

	errMap := sync.Map{}

	as := async.New(ctx)
	for domain, tos := range toByDomain {
		domain := domain
		tos := tos
		mxLookupErr := mxLookupErrors[domain]
		as.WaitProcess(func(p any) {
			recordFailure := func(err error) {
				err = deliveryFailureCause(mxLookupErr, err)
				log.WithContext(ctx).Errorf("%v 邮件投递失败%+v", tos, err)

				errEmailAddressMu.Lock()
				for _, user := range tos {
					errEmailAddress = append(errEmailAddress, user.EmailAddress)
				}
				errEmailAddressMu.Unlock()

				errMap.Store(domain.domain, err)
			}

			if domain.domain == "localhost" {
				err := smtp.SendMailUnsafe("", domain.mxHost+":25", nil, from, fromDomain, buildAddress(tos), data)
				if err != nil {
					recordFailure(err)
				}
				return
			}

			// 获取该域名的全部 MX 主机（按优先级排序），用于故障转移
			mxHosts := mxFailoverMap[domain.domain]
			if len(mxHosts) == 0 {
				mxHosts = []string{domain.mxHost}
			}

			// 按优先级逐个尝试 MX 主机（RFC 5321 §5.1）
			var lastErr error
			for i, mxHost := range mxHosts {
				if i > 0 {
					log.WithContext(ctx).Infof("MX %s 投递失败，尝试下一个 MX: %s (%d/%d)", mxHosts[i-1], mxHost, i+1, len(mxHosts))
				}

				err := tryDeliverToMX(ctx, mxHost, domain.domain, from, fromDomain, buildAddress(tos), data)
				if err == nil {
					return
				}
				lastErr = err

				// 5xx 永久错误是收件人/邮箱层面的拒绝，换 MX 也不会成功
				if isPermanentSMTPResponse(err) {
					recordFailure(err)
					return
				}
			}

			recordFailure(lastErr)
		}, nil)
	}
	as.Wait()

	orgMap := map[string]error{}
	errMap.Range(func(key, value any) bool {
		if value != nil {
			orgMap[key.(string)] = value.(error)
		} else {
			orgMap[key.(string)] = nil
		}

		return true
	})

	if len(errEmailAddress) > 0 {
		return errors.New("以下收件人投递失败：" + array.Join(errEmailAddress, ",")), orgMap
	}
	return nil, orgMap
}

func isPermanentSMTPResponse(err error) bool {
	var protocolErr *textproto.Error
	return errors.As(err, &protocolErr) && protocolErr.Code >= 500 && protocolErr.Code <= 599
}

// tryDeliverToMX 尝试向单个 MX 主机投递，按原有策略依次尝试
// STARTTLS:25 → 587 → 465 → 明文:25
func tryDeliverToMX(ctx *context.Context, mxHost, domain, from, fromDomain string, to []string, data []byte) error {
	// 优先尝试25端口，starttls方式投递
	err := smtp.SendMail("", mxHost+":25", nil, from, fromDomain, to, data)
	if err == nil {
		return nil
	}
	// 证书错误，从新选取证书发送
	var certificateErr *tls.CertificateVerificationError
	if errors.As(err, &certificateErr) {
		var hostnameErr x509.HostnameError
		if errors.As(certificateErr.Err, &hostnameErr) {
			if hostnameErr.Certificate != nil {
				certificateHostName := hostnameErr.Certificate.DNSNames
				err = smtp.SendMail(domainMatch(domain, certificateHostName), mxHost+":25", nil, from, fromDomain, to, data)
			}
		}
	}
	if err == nil {
		return nil
	}
	if isPermanentSMTPResponse(err) {
		return err
	}
	log.WithContext(ctx).Infof("SMTP STARTTLS on 25 Send Error. %s", err.Error())

	// 再试用587投递
	err = smtp.SendMailWithTls("", mxHost+":587", nil, from, fromDomain, to, data)
	if err == nil {
		return nil
	}
	if isPermanentSMTPResponse(err) {
		return err
	}
	log.WithContext(ctx).Infof("SMTPS on 587 Send Error. %s", err.Error())

	// 再次尝试465投递
	err = smtp.SendMailWithTls("", mxHost+":465", nil, from, fromDomain, to, data)
	if err == nil {
		return nil
	}
	if isPermanentSMTPResponse(err) {
		return err
	}
	log.WithContext(ctx).Infof("SMTPS on 465 Send Error. %s", err.Error())

	// 最后尝试非安全方式投递
	err = smtp.SendMailUnsafe("", mxHost+":25", nil, from, fromDomain, to, data)
	if err == nil {
		log.WithContext(ctx).Warnf("Send By Unsafe SMTP")
		return nil
	}

	return err
}

func deliveryFailureCause(mxLookupErr, fallbackErr error) error {
	if fallbackErr == nil || isPermanentSMTPResponse(fallbackErr) {
		return fallbackErr
	}

	var lookupDNSErr *net.DNSError
	if !errors.As(mxLookupErr, &lookupDNSErr) || (!lookupDNSErr.IsTimeout && !lookupDNSErr.IsTemporary) {
		return fallbackErr
	}

	var networkErr net.Error
	if !errors.As(fallbackErr, &networkErr) {
		return fallbackErr
	}

	return &temporaryMXFallbackError{
		lookupErr:   mxLookupErr,
		fallbackErr: fallbackErr,
	}
}

func buildAddress(u []*parsemail.User) []string {
	var ret []string

	for _, user := range u {
		ret = append(ret, user.EmailAddress)

	}

	return ret
}

func domainMatch(domain string, dnsNames []string) string {
	if len(dnsNames) == 0 {
		return domain
	}

	secondMatch := ""

	for _, name := range dnsNames {
		if strings.Contains(name, "smtp") {
			secondMatch = name
		}

		if name == domain {
			return name
		}
		if strings.Contains(name, "*") {
			nameArg := strings.Split(name, ".")
			domainArg := strings.Split(domain, ".")
			match := true
			for i := 0; i < len(nameArg); i++ {
				if nameArg[len(nameArg)-1-i] == "*" {
					continue
				}
				if len(domainArg) > i {
					if nameArg[len(nameArg)-1-i] == domainArg[len(domainArg)-1-i] {
						continue
					}
				}
				match = false
				break
			}

			for i := 0; i < len(domainArg); i++ {
				if len(nameArg) > i && nameArg[len(nameArg)-1-i] == domainArg[len(domainArg)-1-i] {
					continue
				}
				if len(nameArg) > i && nameArg[len(nameArg)-1-i] == "*" {
					continue
				}

				match = false
				break
			}
			if match {
				return domain
			}
		}
	}

	if secondMatch != "" {
		return strings.ReplaceAll(secondMatch, "*.", "")
	}

	return strings.ReplaceAll(dnsNames[0], "*.", "")
}
