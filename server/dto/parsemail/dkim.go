package parsemail

import (
	"bytes"
	"strings"

	"github.com/Jinnrry/pmail/services/domain"
	"github.com/emersion/go-msgauth/dkim"
)

func Init() {}

// SignBytes adds a DKIM signature for the domain in from while preserving
// existing headers/signatures in msgData.
func SignBytes(msgData []byte, from string) ([]byte, error) {
	_, host, ok := strings.Cut(from, "@")
	if !ok {
		return msgData, nil
	}
	d, key, found := domain.Signer(host)
	if !found {
		return msgData, nil
	}
	var buf bytes.Buffer
	if err := dkim.Sign(&buf, bytes.NewReader(msgData), &dkim.SignOptions{Domain: d.Name, Selector: d.DkimSelector, Signer: key}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
