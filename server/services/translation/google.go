package translation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const googleEndpoint = "https://translate.googleapis.com/translate_a/single"
const googleBackupEndpoint = "https://clients5.google.com/translate_a/t"
const googleRetryPrimaryAfter = 10 * time.Minute

type googlePreference struct {
	sync.Mutex
	proxy       [32]byte
	backupUntil time.Time
}

func (s *Service) googleChunk(ctx context.Context, c *http.Client, text, lang string, proxy *url.URL) (string, string, error) {
	identity := proxyIdentity(proxy)
	s.google.Lock()
	backup := s.google.proxy == identity && s.now().Before(s.google.backupUntil)
	s.google.Unlock()
	var limited bool
	for _, useBackup := range []bool{backup, !backup} {
		if ctx.Err() != nil {
			return "", "", transportError(ctx.Err())
		}
		q := url.Values{"sl": {"auto"}, "tl": {lang}, "q": {text}}
		endpoint, parse := s.end.google, parseGTX
		if useBackup {
			endpoint, parse = s.end.googleBackup, parseClients5
			q.Set("client", "dict-chrome-ex")
		} else {
			q.Set("client", "gtx")
			q.Set("dt", "t")
		}
		data, _, err := exchange(ctx, c, http.MethodGet, withQuery(endpoint, q), nil, nil)
		if err == nil {
			translated, source := parse(data)
			if strings.TrimSpace(translated) != "" {
				s.google.Lock()
				if useBackup {
					now := s.now()
					// Do not slide the deadline on every successful backup chunk.
					if s.google.proxy != identity || !now.Before(s.google.backupUntil) {
						s.google.proxy = identity
						s.google.backupUntil = now.Add(googleRetryPrimaryAfter)
					}
				} else if s.google.proxy == identity {
					s.google.backupUntil = time.Time{}
				}
				s.google.Unlock()
				return translated, source, nil
			}
		}
		limited = limited || errors.Is(err, ErrUpstreamRate)
	}
	if ctx.Err() != nil {
		return "", "", transportError(ctx.Err())
	}
	if limited {
		return "", "", ErrUpstreamRate
	}
	return "", "", ErrProvider
}

func parseGTX(data []byte) (string, string) {
	var reply []json.RawMessage
	var rows [][]json.RawMessage
	if json.Unmarshal(data, &reply) != nil || len(reply) == 0 || json.Unmarshal(reply[0], &rows) != nil || len(rows) == 0 {
		return "", ""
	}
	var out strings.Builder
	for _, row := range rows {
		var line *string
		if len(row) == 0 || json.Unmarshal(row[0], &line) != nil || line == nil {
			return "", ""
		}
		out.WriteString(*line)
	}
	var source string
	if len(reply) > 2 {
		_ = json.Unmarshal(reply[2], &source)
	}
	return out.String(), source
}

func parseClients5(data []byte) (string, string) {
	// Auto-detect returns [translation, language] pairs, one per segment.
	// A single flat pair is also accepted when the provider omits the wrapper.
	var rows [][]string
	if json.Unmarshal(data, &rows) != nil {
		var pair []string
		if json.Unmarshal(data, &pair) != nil || len(pair) != 2 {
			return "", ""
		}
		rows = [][]string{pair}
	}
	var out strings.Builder
	source := ""
	for _, pair := range rows {
		if len(pair) != 2 || pair[0] == "" || pair[1] == "" {
			return "", ""
		}
		out.WriteString(pair[0])
		if source == "" {
			source = pair[1]
		}
	}
	return out.String(), source
}
