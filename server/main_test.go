//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/signal"
	"github.com/Jinnrry/pmail/testschema"
)

const (
	testPort          = 17888
	testDomain        = "test.domain"
	testAdmin         = "testcase"
	testAdminPassword = "integration-admin-password"
)

var testHost = "http://127.0.0.1:" + strconv.Itoa(testPort)

func TestMain(m *testing.M) {
	flag.Parse()
	args := flag.Args()
	var dbType, dsn string
	switch {
	case slices.Contains(args, "mysql"):
		dbType, dsn = config.DBTypeMySQL, os.Getenv("PMAIL_TEST_MYSQL_DSN")
	case slices.Contains(args, "postgres") || os.Getenv("PMAIL_TEST_PG_DSN") != "":
		dbType, dsn = config.DBTypePostgres, os.Getenv("PMAIL_TEST_PG_DSN")
	default:
		dbType = config.DBTypeSQLite
	}
	if dbType != config.DBTypeSQLite && dsn == "" {
		fmt.Fprintf(os.Stderr, "Skipping full-server integration: no test DSN for %s\n", dbType)
		os.Exit(0)
	}
	os.Exit(runIntegration(m, dbType, dsn))
}

func runIntegration(m *testing.M, dbType, dsn string) int {
	root, err := os.MkdirTemp("", "pmail-server-integration-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Create integration fixture:", err)
		return 1
	}
	defer os.RemoveAll(root)
	config.ROOT_PATH = root + string(os.PathSeparator)
	if dbType == config.DBTypeSQLite {
		dsn = filepath.Join(root, "config", "integration.db")
	}
	if _, _, err := testschema.WriteSelfSignedCertificate(filepath.Join(root, "config", "ssl"),
		[]string{testDomain, "mail." + testDomain, "smtp." + testDomain, "imap." + testDomain, "pop." + testDomain}, nil); err != nil {
		fmt.Fprintln(os.Stderr, "Create integration certificate:", err)
		return 1
	}
	// Bring up the normal application directly. The legacy setup password
	// writer is intentionally unavailable in v4.
	for key, value := range map[string]string{
		"PMAIL_DB_TYPE": dbType, "PMAIL_DB_DSN": dsn,
		"PMAIL_DOMAIN": testDomain, "PMAIL_DOMAINS": "", "PMAIL_WEB_DOMAIN": "mail." + testDomain,
		"PMAIL_WEB_MODE": "proxy", "PMAIL_HTTP_PORT": strconv.Itoa(testPort),
		"PMAIL_HTTPS_ENABLED": "2", "PMAIL_SSL_TYPE": config.SSLTypeUser,
		"PMAIL_ADMIN_ACCOUNT": testAdmin, "PMAIL_ADMIN_PASSWORD": testAdminPassword,
		"PMAIL_CATCH_ALL": "true", "PMAIL_SPAM_FILTER_LEVEL": "0", "PMAIL_DEFAULT_LANG": "zh-CN",
		"PMAIL_WEB_ROOT": filepath.Join(root, "web"),
	} {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintln(os.Stderr, "Configure integration environment:", err)
			return 1
		}
	}
	stopped := make(chan struct{})
	go func() { defer close(stopped); main() }()
	if !waitForIntegration(stopped) {
		fmt.Fprintln(os.Stderr, "Integration /api/ping not ready within 75 seconds; check the startup log and isolated database")
		return 1
	}
	code := m.Run()
	select {
	case signal.StopChan <- true:
	case <-stopped:
		return code
	case <-time.After(5 * time.Second):
		fmt.Fprintln(os.Stderr, "Integration server did not accept the stop signal")
		return 1
	}
	select {
	case <-stopped:
		return code
	case <-time.After(10 * time.Second):
		fmt.Fprintln(os.Stderr, "Timed out waiting for integration server to stop")
		return 1
	}
}

func waitForIntegration(stopped <-chan struct{}) bool {
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(75 * time.Second)
	for time.Now().Before(deadline) {
		if res, err := client.Get(testHost + "/api/ping"); err == nil {
			data, decodeErr := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode == http.StatusOK && decodeErr == nil && bytes.Contains(data, []byte(`"pong"`)) {
				return true
			}
		}
		select {
		case <-stopped:
			return false
		case <-time.After(100 * time.Millisecond):
		}
	}
	return false
}

func testClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, Timeout: 20 * time.Second}
}

type integrationResult struct {
	Status int
	Body   []byte
}

func integrationRequest(t *testing.T, client *http.Client, method, path string, body any, bearer string) integrationResult {
	t.Helper()
	var input []byte
	if body != nil {
		var err error
		input, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("encode %s: %v", path, err)
		}
	}
	req, err := http.NewRequest(method, testHost+path, bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("%s %s: read: %v; partial body: %s", method, path, err, raw)
	}
	return integrationResult{res.StatusCode, raw}
}

type integrationEnvelope struct {
	ErrorNo int             `json:"errorNo"`
	Data    json.RawMessage `json:"data"`
}

func requireResponse(t *testing.T, path string, got integrationResult, status, errorNo int, data any) {
	t.Helper()
	var env integrationEnvelope
	if err := json.Unmarshal(got.Body, &env); err != nil || got.Status != status || env.ErrorNo != errorNo || len(env.Data) == 0 {
		t.Fatalf("%s: HTTP %d with errorNo %d; want %d/%d; body: %s", path, got.Status, env.ErrorNo, status, errorNo, got.Body)
	}
	if data != nil {
		if err := json.Unmarshal(env.Data, data); err != nil {
			t.Fatalf("%s: unexpected response data for %T: %v; body: %s", path, data, err, got.Body)
		}
	}
}

func postIntegration(t *testing.T, client *http.Client, path string, payload, target any) integrationResult {
	t.Helper()
	res := integrationRequest(t, client, http.MethodPost, path, payload, "")
	requireResponse(t, path, res, http.StatusOK, 0, target)
	return res
}

func testPwdSet(t *testing.T, client *http.Client) {
	t.Helper()
	result := integrationRequest(t, client, http.MethodGet, "/api/bootstrap", nil, "")
	var setup struct {
		NeedRegister *bool `json:"need_register"`
		LoggedIn     *bool `json:"logged_in"`
	}
	if err := json.Unmarshal(result.Body, &setup); err != nil || result.Status != 200 || setup.NeedRegister == nil || *setup.NeedRegister || setup.LoggedIn == nil || *setup.LoggedIn {
		t.Fatalf("explicit-password bootstrap: HTTP %d; body: %s", result.Status, result.Body)
	}
	result = integrationRequest(t, client, http.MethodPost, "/api/register", struct{}{}, "")
	requireResponse(t, "/api/register", result, http.StatusForbidden, response.NoAccessPrivileges, nil)
	if _, err := os.Stat(filepath.Join(config.ROOT_PATH, "config", "registration_key")); !os.IsNotExist(err) {
		t.Fatalf("explicit-password startup must not create a registration key; stat error = %v", err)
	}
}

func testLogin(t *testing.T, client *http.Client, account, password string, admin int) {
	t.Helper()
	var user struct {
		Account string `json:"account"`
		Admin   int    `json:"is_admin"`
	}
	res := postIntegration(t, client, "/api/login", map[string]string{"account": account, "password": password}, &user)
	if user.Account != account || user.Admin != admin {
		t.Fatalf("login identity mismatch: %s", res.Body)
	}
}

type integrationMailItem struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

func mailList(t *testing.T, client *http.Client, bearer string, tag dto.SearchTag) []integrationMailItem {
	t.Helper()
	path := "/api/email/list"
	res := integrationRequest(t, client, http.MethodPost, path, map[string]any{"tag": tag.ToString(), "page_size": 100}, bearer)
	var page struct {
		List []integrationMailItem `json:"list"`
	}
	requireResponse(t, path, res, http.StatusOK, 0, &page)
	for _, item := range page.List {
		if item.ID <= 0 {
			t.Fatalf("mail list entry lacks an ID: %s", res.Body)
		}
	}
	return page.List
}

func findMail(t *testing.T, messages []integrationMailItem, title string) int {
	t.Helper()
	for _, mail := range messages {
		if mail.Title == title {
			return mail.ID
		}
	}
	t.Fatalf("mail %q not in decoded mailbox response: %+v", title, messages)
	return 0
}

func sendIntegrationMail(t *testing.T, client *http.Client, sender, subject string, recipients ...string) integrationResult {
	t.Helper()
	to := make([]map[string]string, 0, len(recipients))
	for _, recipient := range recipients {
		to = append(to, map[string]string{"email": recipient + "@" + testDomain})
	}
	return integrationRequest(t, client, http.MethodPost, "/api/email/send", map[string]any{
		"from_address": sender + "@" + testDomain, "to": to, "subject": subject, "text": "integration message",
	}, "")
}

func TestMaster(t *testing.T) {
	admin := testClient(t)
	if !t.Run("testPwdSet", func(t *testing.T) { testPwdSet(t, admin) }) {
		t.FailNow()
	}
	if !t.Run("testLogin", func(t *testing.T) { testLogin(t, admin, testAdmin, testAdminPassword, 1) }) {
		t.FailNow()
	}

	if !t.Run("testDomainConflict", func(t *testing.T) {
		const path = "/api/domain/create"
		res := integrationRequest(t, admin, http.MethodPost, path, map[string]string{"name": "TEST.DOMAIN."}, "")
		requireResponse(t, path, res, http.StatusConflict, response.ParamsError, nil)
		var env struct {
			ErrorMsg string `json:"errorMsg"`
		}
		if err := json.Unmarshal(res.Body, &env); err != nil || env.ErrorMsg == "" || env.ErrorMsg != i18n.Text(i18n.Default(), "domain_exists") {
			t.Fatalf("normalized duplicate domain: body: %s", res.Body)
		}
	}) {
		t.FailNow()
	}

	if !t.Run("testDraftSaveTwice", func(t *testing.T) { testDraftSaveTwice(t, admin) }) {
		t.FailNow()
	}

	inbox := dto.SearchTag{Type: 0, Status: -1, GroupId: 0}
	const sendPath = "/api/email/send"
	requireResponse(t, sendPath, sendIntegrationMail(t, admin, testAdmin, "Catch-all fixture", "unassigned"), 200, 0, nil)
	initialID := findMail(t, mailList(t, admin, "", inbox), "Catch-all fixture")
	var detail struct {
		ID      int    `json:"id"`
		Subject string `json:"subject"`
	}
	res := postIntegration(t, admin, "/api/email/detail", map[string]int{"id": initialID}, &detail)
	if detail.ID != initialID || detail.Subject != "Catch-all fixture" {
		t.Fatalf("mail detail mismatch: %s", res.Body)
	}
	postIntegration(t, admin, "/api/email/del", map[string][]int{"ids": {initialID}}, nil)
	if deletedID := findMail(t, mailList(t, admin, "", dto.SearchTag{Type: -1, Status: 3, GroupId: -1}), "Catch-all fixture"); deletedID != initialID {
		t.Fatalf("deleted id = %d, want %d", deletedID, initialID)
	}

	for _, account := range []string{"user1", "user2", "user3"} {
		postIntegration(t, admin, "/api/user/create", map[string]string{"account": account, "display_name": account, "password": account}, nil)
	}
	postIntegration(t, admin, "/api/user/edit", map[string]any{"account": "user2", "display_name": "User Two", "password": "user2New"}, nil)
	postIntegration(t, admin, "/api/user/edit", map[string]any{"account": "user3", "disabled": 1}, nil)
	disabled := testClient(t)
	requireResponse(t, "/api/login", integrationRequest(t, disabled, http.MethodPost, "/api/login", map[string]string{"account": "user3", "password": "user3"}, ""), 200, response.ParamsError, nil)
	requireResponse(t, sendPath, sendIntegrationMail(t, admin, testAdmin, "Disabled recipient", "user3"), 200, response.ServerError, nil)

	for _, fixture := range []struct {
		subject string
		to      []string
	}{{"User one only", []string{"user1"}}, {"Shared fixture", []string{"user1", "user2"}}, {"User two only", []string{"user2"}}} {
		requireResponse(t, sendPath, sendIntegrationMail(t, admin, testAdmin, fixture.subject, fixture.to...), 200, 0, nil)
	}
	user1, user2 := testClient(t), testClient(t)
	testLogin(t, user1, "user1", "user1", 0)
	testLogin(t, user2, "user2", "user2New", 0)
	if !t.Run("testSendEmail2User2ForSpam", func(t *testing.T) {
		var sent struct {
			Status string `json:"status"`
		}
		res := postIntegration(t, user2, sendPath, map[string]any{
			"from_address": "user2@" + testDomain, "to": []map[string]string{{"email": testAdmin + "@" + testDomain}},
			"subject": "spam", "text": "NeedMove", "html": "<div>text</div>",
		}, &sent)
		if sent.Status != "sent" {
			t.Fatalf("known-recipient message was not delivered: %s", res.Body)
		}
		findMail(t, mailList(t, admin, "", inbox), "spam")
	}) {
		t.FailNow()
	}
	user2Inbox := mailList(t, user2, "", inbox)
	findMail(t, user2Inbox, "User two only")
	sharedID := findMail(t, user2Inbox, "Shared fixture")
	postIntegration(t, user2, "/api/email/del", map[string][]int{"ids": {sharedID}}, nil)
	findMail(t, mailList(t, user1, "", inbox), "Shared fixture")
	findMail(t, mailList(t, user2, "", dto.SearchTag{Type: -1, Status: 3, GroupId: -1}), "Shared fixture")

	var group int
	res = postIntegration(t, user2, "/api/group/add", map[string]string{"name": "TestGroup"}, &group)
	if group <= 0 {
		t.Fatalf("group ID missing: %s", res.Body)
	}
	var groupList []struct {
		ID int `json:"id"`
	}
	res = postIntegration(t, user2, "/api/group/list", struct{}{}, &groupList)
	if !slices.ContainsFunc(groupList, func(g struct {
		ID int `json:"id"`
	}) bool {
		return g.ID == group
	}) {
		t.Fatalf("group not listed: %s", res.Body)
	}
	postIntegration(t, user2, "/api/rule/add", map[string]any{
		"name": "Move Group", "rules": []map[string]string{{"field": "Subject", "type": "contains", "rule": "Move"}}, "action": 4, "params": strconv.Itoa(group), "sort": 1,
	}, nil)
	var ruleList []struct {
		Action int `json:"action"`
	}
	res = postIntegration(t, user2, "/api/rule/get", struct{}{}, &ruleList)
	if len(ruleList) != 1 || ruleList[0].Action != 4 {
		t.Fatalf("move rule missing: %s", res.Body)
	}
	requireResponse(t, sendPath, sendIntegrationMail(t, admin, testAdmin, "Move fixture", "user2"), 200, 0, nil)
	findMail(t, mailList(t, user2, "", dto.SearchTag{Type: -1, Status: -1, GroupId: group}), "Move fixture")
	for i := range 10 {
		requireResponse(t, sendPath, sendIntegrationMail(t, admin, testAdmin, fmt.Sprintf("Batch %d", i), "user2"), 200, 0, nil)
	}

	var issued struct {
		Token string `json:"token"`
	}
	postIntegration(t, user2, "/api/token/create", map[string]string{"name": "test-token"}, &issued)
	if issued.Token == "" {
		t.Fatal("token create response contained no token")
	}
	findMail(t, mailList(t, testClient(t), issued.Token, inbox), "Batch 9")
}

// Uses the same configured backend as TestMaster, including PostgreSQL.
func testDraftSaveTwice(t *testing.T, client *http.Client) {
	const save = "/api/email/draft/save"
	var first, second struct {
		ID int `json:"id"`
	}
	res := postIntegration(t, client, save, map[string]string{"subject": "initial draft", "text": "before"}, &first)
	if first.ID <= 0 {
		t.Fatalf("draft ID not assigned: %s", res.Body)
	}
	t.Cleanup(func() {
		postIntegration(t, client, "/api/email/draft/delete", map[string]int{"id": first.ID}, nil)
	})
	res = postIntegration(t, client, save, map[string]any{"id": first.ID, "subject": "revised draft", "text": "after"}, &second)
	if second.ID != first.ID {
		t.Fatalf("draft update changed ID %d: %s", first.ID, res.Body)
	}
	var detail struct {
		ID      int    `json:"id"`
		Subject string `json:"subject"`
		Text    string `json:"text"`
	}
	res = postIntegration(t, client, "/api/email/detail", map[string]int{"id": first.ID}, &detail)
	if detail.ID != first.ID || detail.Subject != "revised draft" || detail.Text != "after" {
		t.Fatalf("draft content not updated: %s", res.Body)
	}
}
