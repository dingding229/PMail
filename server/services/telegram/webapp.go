package telegram

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	pctx "github.com/Jinnrry/pmail/utils/context"
)

// The Telegram Mini App shows one pushed mail inside Telegram. Telegram signs
// the launch data with the bot token; the server checks that signature, that
// the Telegram user is the private chat bound to a PMail account, and that the
// account owns the mail. It then hands out a short-lived ticket that only lets
// the page load that mail's images and attachments. No login session or API
// token is created, and nothing can be sent or changed from the page.

var (
	ErrWebAppAuth   = errors.New("telegram_webapp_auth")
	ErrWebAppTicket = errors.New("telegram_webapp_ticket")
)

const (
	webAppMaxAge  = time.Hour
	webAppSkew    = 5 * time.Minute
	webAppTicket  = 30 * time.Minute
	webAppMaxData = 8192
)

// WebAppBase returns the public HTTPS origin used for Mini App buttons, or
// "" when none is configured. Telegram only opens Mini Apps over HTTPS.
func WebAppBase() string {
	if u, err := url.Parse(strings.TrimSpace(os.Getenv("PMAIL_PUBLIC_URL"))); err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil {
		return "https://" + u.Host
	}
	if c := config.Instance; c != nil && c.WebDomain != "" && c.HttpsEnabled != 2 {
		host := c.WebDomain
		if c.HttpsPort != 0 && c.HttpsPort != 443 {
			host += ":" + strconv.Itoa(c.HttpsPort)
		}
		return "https://" + host
	}
	return ""
}

func webAppURL(id int) string {
	base := WebAppBase()
	if base == "" || id <= 0 {
		return ""
	}
	return base + "/tg/mail?mail=" + strconv.Itoa(id)
}

// verifyInitData checks Telegram's signature over the Mini App launch data and
// returns the Telegram user ID. See core.telegram.org/bots/webapps.
func verifyInitData(raw, botToken string, now time.Time) (int64, error) {
	if raw == "" || len(raw) > webAppMaxData || botToken == "" {
		return 0, ErrWebAppAuth
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return 0, ErrWebAppAuth
	}
	got := values.Get("hash")
	if len(values["hash"]) != 1 || len(got) != 64 {
		return 0, ErrWebAppAuth
	}
	keys := make([]string, 0, len(values))
	for k, v := range values {
		if len(v) != 1 {
			return 0, ErrWebAppAuth
		}
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + values.Get(k)
	}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))
	want, err := hex.DecodeString(got)
	if err != nil || !hmac.Equal(mac.Sum(nil), want) {
		return 0, ErrWebAppAuth
	}
	auth, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil {
		return 0, ErrWebAppAuth
	}
	at := time.Unix(auth, 0)
	if now.Sub(at) > webAppMaxAge || at.Sub(now) > webAppSkew {
		return 0, ErrWebAppAuth
	}
	var user struct {
		ID int64 `json:"id"`
	}
	if json.Unmarshal([]byte(values.Get("user")), &user) != nil || user.ID <= 0 {
		return 0, ErrWebAppAuth
	}
	return user.ID, nil
}

// VerifyWebApp authenticates a Mini App request for one mail and returns the
// PMail user ID that owns it.
func VerifyWebApp(ctx context.Context, initData string, mailID int) (int, string, error) {
	if mailID <= 0 {
		return 0, "", ErrWebAppAuth
	}
	var c models.TelegramConfig
	ok, err := db.Instance.Context(ctx).ID(1).Get(&c)
	if err != nil {
		return 0, "", err
	}
	if !ok || c.Token == "" || c.Generation == "" {
		return 0, "", ErrWebAppAuth
	}
	token, err := unseal(c.Token, "token")
	if err != nil {
		return 0, "", err
	}
	tgUser, err := verifyInitData(initData, token, time.Now())
	if err != nil {
		return 0, "", err
	}
	// In a private chat the chat ID is the user's ID; group bindings never
	// match, so only the bound person can open the mail.
	b, lang, ok, err := allowedCallback(ctx, tgUser, c.Generation, mailID)
	if err != nil {
		return 0, "", err
	}
	if !ok || b.ChatID != tgUser {
		return 0, "", ErrWebAppAuth
	}
	return b.UserID, lang, nil
}

func ticketKey() []byte {
	secret := ""
	if config.Instance != nil {
		secret = config.Instance.SecretKey
	}
	sum := sha256.Sum256([]byte("pmail-telegram-webapp\x00" + secret))
	return sum[:]
}

func ticketMAC(uid, mailID int, exp int64) string {
	mac := hmac.New(sha256.New, ticketKey())
	mac.Write([]byte(strconv.Itoa(uid) + ":" + strconv.Itoa(mailID) + ":" + strconv.FormatInt(exp, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:20])
}

// Ticket returns a value that lets the Mini App page load one mail's files.
func Ticket(uid, mailID int, now time.Time) string {
	exp := now.Add(webAppTicket).Unix()
	return strconv.FormatInt(exp, 10) + "." + strconv.Itoa(uid) + "." + ticketMAC(uid, mailID, exp)
}

// CheckTicket returns the user ID a ticket was issued to, after checking its
// signature, its expiry and that the user still owns the mail.
func CheckTicket(ctx context.Context, ticket string, mailID int, now time.Time) (int, error) {
	parts := strings.Split(ticket, ".")
	if len(parts) != 3 || mailID <= 0 {
		return 0, ErrWebAppTicket
	}
	exp, err1 := strconv.ParseInt(parts[0], 10, 64)
	uid, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || uid <= 0 || now.Unix() > exp || exp-now.Unix() > int64(webAppTicket/time.Second) {
		return 0, ErrWebAppTicket
	}
	if !hmac.Equal([]byte(parts[2]), []byte(ticketMAC(uid, mailID, exp))) {
		return 0, ErrWebAppTicket
	}
	ok, err := db.Instance.Context(ctx).SQL(`SELECT 1 FROM user_email ue JOIN `+db.Instance.Quote("user")+` u ON u.id=ue.user_id
		WHERE ue.user_id=? AND ue.email_id=? AND u.disabled=0`, uid, mailID).Exist()
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ErrWebAppTicket
	}
	return uid, nil
}

// withWebApp adds a "view mail" Mini App button. Telegram only allows Mini
// App buttons in private chats, which have positive chat IDs.
func withWebApp(ctx *pctx.Context, markup *replyMarkup, chatID int64, mailID int) *replyMarkup {
	link := webAppURL(mailID)
	if chatID <= 0 || link == "" {
		return markup
	}
	if markup == nil {
		markup = &replyMarkup{}
	}
	markup.Keyboard = append([][]inlineButton{{{Text: i18n.T(ctx, "telegram_view_mail"), WebApp: &webAppButton{URL: link}}}}, markup.Keyboard...)
	return markup
}
