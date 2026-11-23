package http_server

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"

	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/token"
)

func fixtureFactorCode(t *testing.T, secret string) string {
	t.Helper()
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha1.New, decoded)
	if err := binary.Write(mac, binary.BigEndian, time.Now().Unix()/30); err != nil {
		t.Fatal(err)
	}
	value := mac.Sum(nil)
	index := value[len(value)-1] & 15
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(value[index:])&0x7fffffff)%1_000_000)
}

func TestSecondFactorHTTPLoginAndAdministratorReset(t *testing.T) {
	f := newAuthFixture(t)
	cfg := &config.Config{Domain: "factor-http.test", SecretKey: "test-only fixture key"}
	old := config.Instance
	config.Instance = cfg
	t.Cleanup(func() { config.Instance = old })
	if err := db.SyncSchema(f.db); err != nil {
		t.Fatal(err)
	}
	if err := domain.Initialize(cfg); err != nil {
		t.Fatal(err)
	}
	primary := models.MailboxAddress{UserID: f.member.ID, DomainID: domain.Default().ID, LocalPart: "member", IsPrimary: true}
	if _, err := f.db.Insert(&primary); err != nil {
		t.Fatal(err)
	}
	member := f.login(t, "member", "initial-password")
	oldMember := f.login(t, "member", "initial-password")
	admin := f.login(t, "admin", "initial-password")
	_, clientToken, err := token.Create(f.member.ID, "2fa-fixture", token.Read, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	setup := f.call(t, member, "/api/2fa/setup", map[string]any{})
	if setup.ErrorNo != 0 {
		t.Fatal(setup)
	}
	secret := setup.Data.(map[string]any)["secret"].(string)
	enabled := f.call(t, member, "/api/2fa/enable", map[string]any{"code": fixtureFactorCode(t, secret)})
	if enabled.ErrorNo != 0 {
		t.Fatal(enabled)
	}
	codes := enabled.Data.(map[string]any)["recovery_codes"].([]any)
	if len(codes) != 10 {
		t.Fatalf("%d recovery codes", len(codes))
	}
	if res := f.call(t, oldMember, "/api/user/info", nil); res.ErrorNo != response.NeedLogin {
		t.Fatal("pre-2FA session was not revoked")
	}
	if res := f.call(t, member, "/api/user/info", nil); res.ErrorNo != 0 {
		t.Fatal("setup session was not rotated")
	}

	if tokenUser, _, err := token.Authenticate(clientToken); err != nil || tokenUser.ID != f.member.ID {
		t.Fatalf("enabling 2FA changed the existing token policy: %v", err)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	fresh := &http.Client{Jar: jar}
	loginBody := map[string]string{"account": "member", "password": "initial-password"}
	first := f.call(t, fresh, "/api/login", loginBody)
	if first.ErrorNo != 0 {
		t.Fatal(first)
	}
	challengeData := first.Data.(map[string]any)
	if challengeData["two_factor_required"] != true || challengeData["expires_in"] != float64(300) {
		t.Fatal("missing challenge metadata")
	}
	serverURL, err := url.Parse(f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(jar.Cookies(serverURL)) != 0 {
		t.Fatal("first step issued a session cookie")
	}
	if res := f.call(t, fresh, "/api/user/info", nil); res.ErrorNo != response.NeedLogin {
		t.Fatal("first step authenticated")
	}
	factorBody := map[string]any{"challenge": challengeData["challenge"], "recovery": true, "code": "INVALID"}
	if res := f.call(t, fresh, "/api/login/2fa", factorBody); res.ErrorNo == 0 {
		t.Fatal("invalid factor accepted")
	}
	factorBody["code"] = codes[0]
	if res := f.call(t, fresh, "/api/login/2fa", factorBody); res.ErrorNo != 0 {
		t.Fatal(res)
	}
	if res := f.call(t, fresh, "/api/user/info", nil); res.ErrorNo != 0 {
		t.Fatal("second step did not authenticate")
	}
	if res := f.call(t, fresh, "/api/login/2fa", factorBody); res.ErrorNo == 0 {
		t.Fatal("challenge replayed")
	}

	if res := f.call(t, member, "/api/user/reset_2fa", map[string]int{"id": f.admin.ID}); res.ErrorNo == 0 {
		t.Fatal("non-admin reset was allowed")
	}
	if res := f.call(t, admin, "/api/user/reset_2fa", map[string]int{"id": f.admin.ID}); res.ErrorNo == 0 {
		t.Fatal("self reset was allowed")
	}
	if res := f.call(t, admin, "/api/user/reset_2fa", map[string]int{"id": f.member.ID}); res.ErrorNo != 0 {
		t.Fatal(res)
	}
	if res := f.call(t, fresh, "/api/user/info", nil); res.ErrorNo != response.NeedLogin {
		t.Fatal("reset did not revoke session")
	}
	var row models.User
	if ok, err := f.db.ID(f.member.ID).Get(&row); err != nil || !ok || row.TOTPEnabled {
		t.Fatalf("reset flag: %v", err)
	}
	if found, err := f.db.ID(row.ID).Exist(&models.TOTPConfig{}); err != nil || found {
		t.Fatalf("reset secret: %v", err)
	}
	if found, err := f.db.Where("user_id = ?", row.ID).Exist(&models.RecoveryCode{}); err != nil || found {
		t.Fatalf("reset codes: %v", err)
	}
	newLogin := f.call(t, fresh, "/api/login", loginBody)
	if newLogin.ErrorNo != 0 || newLogin.Data.(map[string]any)["account"] != "member" {
		t.Fatal("password login not restored")
	}
}

func TestAppPasswordAPINoStoreAndNoHashInList(t *testing.T) {
	f := newAuthFixture(t)
	if err := db.SyncSchema(f.db); err != nil {
		t.Fatal(err)
	}
	client := f.login(t, "member", "initial-password")
	res := f.call(t, client, "/api/app_passwords", map[string]string{"name": "desktop"})
	if res.ErrorNo != 0 {
		t.Fatal(res)
	}
	data := res.Data.(map[string]any)
	if len(data["password"].(string)) != 16 {
		t.Fatal("not a 16-character credential")
	}
	resp, err := client.Get(f.server.URL + "/api/app_passwords")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Cache-Control"), "no-store") {
		t.Fatal("unexpected list status/cache control")
	}
	var list response.Response
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	row := list.Data.([]any)[0].(map[string]any)
	if _, exists := row["password"]; exists || len(row) != 4 {
		t.Fatal("unexpected fields in app-password list")
	}
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/app_passwords/%.0f", f.server.URL, data["id"]), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", f.server.URL)
	deleted, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer deleted.Body.Close()
	var result response.Response
	if err := json.NewDecoder(deleted.Body).Decode(&result); err != nil || result.ErrorNo != 0 || deleted.StatusCode != http.StatusOK {
		t.Fatalf("delete: %v (%d)", err, deleted.StatusCode)
	}
}
