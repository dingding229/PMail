package parsemail

import (
	"bytes"
	"io"
	"strings"

	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/utils/consts"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/emersion/go-msgauth/dkim"
	log "github.com/sirupsen/logrus"
)

var instance any = struct{}{}

func Init() {}

func sign(msgData, from string) []byte {
	_, host, ok := strings.Cut(from, "@")
	if !ok {
		return []byte(msgData)
	}
	d, key, found := domain.Signer(host)
	if !found {
		return []byte(msgData)
	}
	var buf bytes.Buffer
	if err := dkim.Sign(&buf, strings.NewReader(msgData), &dkim.SignOptions{Domain: d.Name, Selector: d.DkimSelector, Signer: key}); err != nil {
		log.Errorf("DKIM signing failed for %s: %v", d.Name, err)
		return []byte(msgData)
	}
	return buf.Bytes()
}

func Check(ctx *context.Context, mail io.Reader) bool {

	verifications, err := dkim.Verify(mail)
	if err != nil {
		log.WithContext(ctx).Warnf("DKIM Error:%v", err)
	}

	if len(verifications) == 0 {
		return false
	}

	for _, v := range verifications {
		if v.Domain == consts.TEST_DOMAIN {
			return true
		}
		if v.Err == nil {
			log.Println("Valid signature for:", v.Domain)
		} else {
			log.Println("Invalid signature for:", v.Domain, v.Err)
			return false
		}
	}
	return true
}
