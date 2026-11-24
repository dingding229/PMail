package translation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const myMemoryEndpoint = "https://api.mymemory.translated.net/get"

func numericStatus(raw json.RawMessage) int {
	v := strings.TrimSpace(string(raw))
	if len(v) > 0 && v[0] == '"' {
		if json.Unmarshal(raw, &v) != nil {
			return 0
		}
		v = strings.TrimSpace(v)
	}
	n, _ := strconv.Atoi(v)
	return n
}

func (s *Service) myMemoryChunk(ctx context.Context, c *http.Client, text, source, lang, email string) (string, string, error) {
	if source == lang {
		return text, source, nil
	}
	q := url.Values{"q": {text}, "langpair": {source + "|" + lang}}
	if email != "" {
		q.Set("de", email)
	}
	raw, _, err := exchangeResponse(ctx, c, http.MethodGet, withQuery(s.end.myMemory, q), nil, "", nil, true)
	var reply struct {
		Status        json.RawMessage `json:"responseStatus"`
		QuotaFinished bool            `json:"quotaFinished"`
		Data          json.RawMessage `json:"responseData"`
	}
	if json.Unmarshal(raw, &reply) != nil {
		if err != nil {
			return "", "", err
		}
		return "", "", ErrFormat
	}
	status := numericStatus(reply.Status)
	var data struct {
		Text string `json:"translatedText"`
	}
	decoded := json.Unmarshal(reply.Data, &data) == nil
	if reply.QuotaFinished || status == 429 || strings.HasPrefix(strings.ToUpper(strings.TrimSpace(data.Text)), "MYMEMORY WARNING") {
		return "", "", ErrUpstreamRate
	}
	if err != nil {
		return "", "", err
	}
	if status != 0 && status != 200 {
		return "", "", ErrProvider
	}
	if !decoded || strings.TrimSpace(data.Text) == "" {
		return "", "", ErrFormat
	}
	return data.Text, source, nil
}
