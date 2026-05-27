package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func localSMSTestApp(app *App) *App {
	app.allowLocalSMSDebug = true
	return app
}

func TestInviteRegisterSessionLoop(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788", attachmentDir: t.TempDir()})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	invite := postJSON[InviteRecord](t, client, server.URL+"/api/v1/post-office/admin/mail/invites", map[string]any{
		"prefix":        "user",
		"emailPrefix":   "loop",
		"phone":         "13800138009",
		"note":          "test",
		"expiresInDays": 7,
	})
	if invite.Code == "" || invite.Email != "user-loop@yuexiang.com" {
		t.Fatalf("unexpected invite: %#v", invite)
	}

	sms := postJSON[SMSLogRecord](t, client, server.URL+"/api/v1/post-office/auth/sms/send", map[string]any{
		"phone":   "13800138009",
		"purpose": "register",
	})
	if sms.Code == "" {
		t.Fatalf("empty sms code: %#v", sms)
	}

	session := postJSON[response](t, client, server.URL+"/api/v1/post-office/auth/register", map[string]any{
		"phone":       "13800138009",
		"smsCode":     sms.Code,
		"password":    "Passw0rd",
		"displayName": "Loop User",
		"emailPrefix": "loop",
		"prefix":      "user",
		"inviteCode":  invite.Code,
	})
	if session["isAuthenticated"] != true {
		t.Fatalf("session not authenticated: %#v", session)
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/post-office/auth/session", nil)
	if err != nil {
		t.Fatalf("new session request: %v", err)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("session request: %v", err)
	}
	defer res.Body.Close()

	var current response
	if err := json.NewDecoder(res.Body).Decode(&current); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if current["isAuthenticated"] != true {
		t.Fatalf("current session not authenticated: %#v", current)
	}

	audits := getJSON[response](t, client, server.URL+"/api/v1/post-office/admin/mail/audit-logs")
	items, ok := audits["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected audit log items, got %#v", audits)
	}
	if !auditItemsContain(items, "auth.register") {
		t.Fatalf("expected auth.register audit item, got %#v", items)
	}
}

func TestEmailPasswordRegisterLoginLoop(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788", attachmentDir: t.TempDir()}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	client := http.DefaultClient
	config := patchJSON[response](t, client, server.URL+"/api/v1/post-office/admin/mail/config", map[string]any{
		"auth": map[string]any{
			"phoneLoginEnabled": false,
			"emailLoginEnabled": true,
			"inviteRequired":    false,
			"loginLandingMode":  "account",
		},
	})
	auth := config["auth"].(map[string]any)
	if auth["emailLoginEnabled"] != true || auth["phoneLoginEnabled"] != false || auth["loginLandingMode"] != "account" {
		t.Fatalf("expected email auth config, got %#v", auth)
	}

	registered := postJSON[response](t, client, server.URL+"/api/v1/post-office/auth/register", map[string]any{
		"loginType":   "email",
		"identifier":  "user-mail@yuexiang.com",
		"password":    "Passw0rd",
		"displayName": "Mail User",
	})
	if registered["isAuthenticated"] != true {
		t.Fatalf("expected registered session, got %#v", registered)
	}

	loggedIn := postJSON[response](t, client, server.URL+"/api/v1/post-office/auth/login", map[string]any{
		"loginType":  "email",
		"identifier": "user-mail@yuexiang.com",
		"password":   "Passw0rd",
	})
	if loggedIn["isAuthenticated"] != true {
		t.Fatalf("expected email login session, got %#v", loggedIn)
	}
}

func TestAdminTokenProtection(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788", adminAPIToken: "secret-admin-token"}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	res, err := http.Get(server.URL + "/api/v1/post-office/admin/mail/config")
	if err != nil {
		t.Fatalf("unauthorized request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without admin token, got %d", res.StatusCode)
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/post-office/admin/mail/config", nil)
	if err != nil {
		t.Fatalf("new authorized request: %v", err)
	}
	req.Header.Set("X-Admin-Token", "secret-admin-token")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authorized request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with admin token, got %d", res.StatusCode)
	}
}

func TestAdminSecurityConfigProtectsAdminAPI(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	app := &App{
		store:         store,
		userAppOrigin: "http://127.0.0.1:1788",
		adminSessions: map[string]AdminSessionRecord{},
	}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	config := requestJSON[response](t, http.DefaultClient, http.MethodPatch, server.URL+"/api/v1/post-office/admin/mail/config", map[string]any{
		"security": map[string]any{
			"username":    "owner",
			"newPassword": "StrongPass123",
			"apiToken":    "persisted-admin-token",
		},
	}, nil)
	security := config["security"].(map[string]any)
	if security["passwordSet"] != true || security["apiTokenSet"] != true || security["passwordHash"] != nil || security["apiTokenHash"] != nil {
		t.Fatalf("expected redacted persisted security config, got %#v", security)
	}
	if buildDeploymentStatus(store.snapshot().Config, "postgres").Ready {
		t.Fatalf("deployment should still wait for non-security production prerequisites")
	}

	res, err := http.Get(server.URL + "/api/v1/post-office/admin/mail/config")
	if err != nil {
		t.Fatalf("unauthorized request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 after setting stored admin password, got %d", res.StatusCode)
	}

	login := requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/admin/auth/login", map[string]any{
		"username": "owner",
		"password": "StrongPass123",
	}, nil)
	token, ok := login["token"].(string)
	if !ok || !strings.HasPrefix(token, "adm_") {
		t.Fatalf("expected stored admin session token, got %#v", login)
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/post-office/admin/mail/config", nil)
	if err != nil {
		t.Fatalf("new admin request: %v", err)
	}
	req.Header.Set("X-Admin-Token", "persisted-admin-token")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stored admin token request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with stored admin token, got %d", res.StatusCode)
	}
}

func TestAdminPasswordLoginSession(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	app := &App{
		store:         store,
		userAppOrigin: "http://127.0.0.1:1788",
		adminUsername: "owner",
		adminPassword: "strong-password",
		adminSessions: map[string]AdminSessionRecord{},
	}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	status := requestJSONStatus(t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/admin/auth/login", map[string]any{
		"username": "owner",
		"password": "bad-password",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("expected bad admin login to be 401, got %d", status)
	}

	login := requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/admin/auth/login", map[string]any{
		"username": "owner",
		"password": "strong-password",
	}, nil)
	token, ok := login["token"].(string)
	if !ok || !strings.HasPrefix(token, "adm_") {
		t.Fatalf("expected admin session token, got %#v", login)
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/post-office/admin/mail/config", nil)
	if err != nil {
		t.Fatalf("new admin session request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin session request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with admin session, got %d", res.StatusCode)
	}

	_ = requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/admin/auth/logout", map[string]any{}, map[string]string{
		"Authorization": "Bearer " + token,
	})
	req, err = http.NewRequest(http.MethodGet, server.URL+"/api/v1/post-office/admin/mail/config", nil)
	if err != nil {
		t.Fatalf("new logged out admin request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("logged out admin request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 after admin logout, got %d", res.StatusCode)
	}

	auditActions := map[string]bool{}
	for _, item := range store.snapshot().Config.AuditLogs {
		auditActions[item.Action] = true
	}
	for _, action := range []string{"admin.auth.login_failed", "admin.auth.login", "admin.auth.logout"} {
		if !auditActions[action] {
			t.Fatalf("expected admin auth audit action %s, got %#v", action, auditActions)
		}
	}
}

func TestAccountLifecycle(t *testing.T) {
	t.Parallel()

	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/mailboxes/lifecycle":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"externalId":"principal-lifecycle","status":"synced"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mailServer.Close)

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:      "stalwart",
		Enabled:       true,
		BaseURL:       mailServer.URL + "/healthz",
		LifecyclePath: mailServer.URL + "/api/v1/mailboxes/lifecycle",
		Status:        "online",
	}
	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	invite := postJSON[InviteRecord](t, client, server.URL+"/api/v1/post-office/admin/mail/invites", map[string]any{
		"prefix":      "user",
		"emailPrefix": "lifecycle",
		"phone":       "13800138007",
	})
	registerSMS := postJSON[SMSLogRecord](t, client, server.URL+"/api/v1/post-office/auth/sms/send", map[string]any{
		"phone":   "13800138007",
		"purpose": "register",
	})
	session := postJSON[response](t, client, server.URL+"/api/v1/post-office/auth/register", map[string]any{
		"phone":       "13800138007",
		"smsCode":     registerSMS.Code,
		"password":    "OldPassw0rd",
		"displayName": "Lifecycle User",
		"emailPrefix": "lifecycle",
		"prefix":      "user",
		"inviteCode":  invite.Code,
	})
	profile := session["profile"].(map[string]any)
	accountID := profile["id"].(string)

	disabled := postJSON[response](t, client, server.URL+"/api/v1/post-office/admin/mail/accounts/"+accountID+"/disable", map[string]any{})
	disabledAccount := disabled["account"].(map[string]any)
	if disabledAccount["status"] != "disabled" {
		t.Fatalf("expected disabled account, got %#v", disabledAccount)
	}

	current := getJSON[response](t, client, server.URL+"/api/v1/post-office/auth/session")
	if current["isAuthenticated"] != false {
		t.Fatalf("disabled account should not keep session: %#v", current)
	}

	loginSMS := postJSON[SMSLogRecord](t, client, server.URL+"/api/v1/post-office/auth/sms/send", map[string]any{
		"phone":   "13800138007",
		"purpose": "login",
	})
	loginStatus := postJSONStatus(t, client, server.URL+"/api/v1/post-office/auth/login", map[string]any{
		"phone":    "13800138007",
		"smsCode":  loginSMS.Code,
		"password": "OldPassw0rd",
	})
	if loginStatus != http.StatusBadRequest {
		t.Fatalf("disabled account login should fail, got %d", loginStatus)
	}

	enabled := postJSON[response](t, client, server.URL+"/api/v1/post-office/admin/mail/accounts/"+accountID+"/enable", map[string]any{})
	enabledAccount := enabled["account"].(map[string]any)
	if enabledAccount["status"] != "active" {
		t.Fatalf("expected active account, got %#v", enabledAccount)
	}

	reset := postJSON[response](t, client, server.URL+"/api/v1/post-office/admin/mail/accounts/"+accountID+"/reset-password", map[string]any{})
	temporaryPassword, ok := reset["temporaryPassword"].(string)
	if !ok || temporaryPassword == "" {
		t.Fatalf("expected temporary password, got %#v", reset)
	}

	nextSession := postJSON[response](t, client, server.URL+"/api/v1/post-office/auth/login", map[string]any{
		"phone":    "13800138007",
		"smsCode":  loginSMS.Code,
		"password": temporaryPassword,
	})
	if nextSession["isAuthenticated"] != true {
		t.Fatalf("login with temporary password failed: %#v", nextSession)
	}
}

func TestSendMessageStoresSentMail(t *testing.T) {
	t.Parallel()

	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/messages/send":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"providerMessageId":"provider-store-1","acceptedAt":"2026-05-25T02:00:00Z"}}`))
		case "/api/v1/messages":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"items":[],"hasMore":false}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mailServer.Close)

	store, session := newAuthenticatedMessageTestStore(t)
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:        "stalwart",
		Enabled:         true,
		BaseURL:         mailServer.URL,
		MessageSendPath: "/api/v1/messages/send",
		MessageListPath: "/api/v1/messages",
		Status:          "online",
	}
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788", attachmentDir: t.TempDir()}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	headers := map[string]string{"Authorization": "Bearer " + session.Token}
	sent := requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/messages/send", map[string]any{
		"recipients": []string{"buyer@example.com"},
		"subject":    "商业闭环发信测试",
		"body": map[string]any{
			"format": "text",
			"text":   "这是一封通过 BFF 落库的发信测试。",
		},
		"attachments": []any{map[string]any{
			"id":              "att-contract-1",
			"name":            "contract.txt",
			"type":            "txt",
			"contentType":     "text/plain",
			"sizeBytes":       8,
			"sizeLabel":       "8 B",
			"contentEncoding": "base64",
			"contentBase64":   "Y29udHJhY3Q=",
		}},
		"source": "manual",
	}, headers)
	data := sent["data"].(map[string]any)
	message := data["message"].(map[string]any)
	if message["folder"] != "sent" || message["deliveryStatus"] != "accepted" {
		t.Fatalf("expected accepted sent message, got %#v", message)
	}
	if message["acceptedAt"] == "" {
		t.Fatalf("expected acceptedAt on sent message, got %#v", message)
	}
	if message["hasAttachment"] != true {
		t.Fatalf("expected sent message attachment flag, got %#v", message)
	}
	attachments := message["attachments"].([]any)
	if len(attachments) != 1 {
		t.Fatalf("expected persisted attachment metadata, got %#v", attachments)
	}
	attachment := attachments[0].(map[string]any)
	if attachment["contentBase64"] != nil || attachment["assetId"] == nil || attachment["downloadUrl"] == "" {
		t.Fatalf("expected persisted attachment metadata, got %#v", attachment)
	}
	downloadReq, err := http.NewRequest(http.MethodGet, server.URL+attachment["downloadUrl"].(string), nil)
	if err != nil {
		t.Fatalf("new download request: %v", err)
	}
	downloadReq.Header.Set("Authorization", "Bearer "+session.Token)
	downloadRes, err := http.DefaultClient.Do(downloadReq)
	if err != nil {
		t.Fatalf("download attachment: %v", err)
	}
	defer downloadRes.Body.Close()
	if downloadRes.StatusCode != http.StatusOK {
		t.Fatalf("expected attachment download 200, got %d", downloadRes.StatusCode)
	}
	downloaded, _ := io.ReadAll(downloadRes.Body)
	if string(downloaded) != "contract" {
		t.Fatalf("unexpected attachment content %q", string(downloaded))
	}
	if data["acceptedAt"] == "" {
		t.Fatalf("expected accepted timestamp, got %#v", data)
	}

	list := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/messages?folderId=sent", headers)
	listData := list["data"].(map[string]any)
	items := listData["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one sent message, got %#v", listData)
	}
	counts := listData["folderCounts"].(map[string]any)
	if counts["sent"] != float64(1) {
		t.Fatalf("expected sent count to update, got %#v", counts)
	}
}

func TestSaveDraftAndMessageActions(t *testing.T) {
	t.Parallel()

	remoteFolder := "drafts"
	remoteStarred := false
	remoteDraftID := ""
	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/messages/drafts":
			var payload MailboxMessageRelayPayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode draft payload: %v", err)
			}
			remoteDraftID = payload.MessageID
			_ = json.NewEncoder(w).Encode(response{"data": response{"draft": response{
				"id":             remoteDraftID,
				"folder":         "drafts",
				"deliveryStatus": "draft",
			}}})
		case strings.HasSuffix(r.URL.Path, "/star"):
			var payload MailboxMessageActionPayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode star payload: %v", err)
			}
			if payload.Starred != nil {
				remoteStarred = *payload.Starred
			}
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/move"):
			var payload MailboxMessageActionPayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode move payload: %v", err)
			}
			remoteFolder = payload.TargetFolder
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(r.URL.Path, "/api/v1/messages/"):
			messageID := strings.TrimPrefix(r.URL.Path, "/api/v1/messages/")
			if remoteDraftID != "" {
				messageID = remoteDraftID
			}
			_ = json.NewEncoder(w).Encode(response{"data": response{"message": response{
				"id":             messageID,
				"folder":         remoteFolder,
				"sender":         "Message User",
				"senderEmail":    "message-user@yuexiang.com",
				"recipients":     []string{"partner@example.com"},
				"subject":        "草稿链路测试",
				"content":        "<p>草稿也必须经过 BFF 保存。</p>",
				"isStarred":      remoteStarred,
				"deliveryStatus": "draft",
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mailServer.Close)

	store, session := newAuthenticatedMessageTestStore(t)
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:          "stalwart",
		Enabled:           true,
		BaseURL:           mailServer.URL,
		DraftPath:         "/api/v1/messages/drafts",
		MessageDetailPath: "/api/v1/messages/{messageId}",
		MessageStarPath:   "/api/v1/messages/{messageId}/star",
		MessageMovePath:   "/api/v1/messages/{messageId}/move",
		Status:            "online",
	}
	store.normalizeLocked()

	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	headers := map[string]string{"Authorization": "Bearer " + session.Token}
	saved := requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/drafts", map[string]any{
		"recipients": []string{"partner@example.com"},
		"subject":    "草稿链路测试",
		"body": map[string]any{
			"format": "text",
			"text":   "草稿也必须经过 BFF 保存。",
		},
		"attachments": []any{},
		"autosave":    false,
	}, headers)
	draft := saved["data"].(map[string]any)["draft"].(map[string]any)
	draftID := draft["id"].(string)
	if draft["folder"] != "drafts" || draft["deliveryStatus"] != "draft" {
		t.Fatalf("expected saved draft, got %#v", draft)
	}

	starred := requestJSON[response](t, http.DefaultClient, http.MethodPatch, server.URL+"/api/v1/post-office/messages/"+draftID+"/star", map[string]any{
		"starred": true,
	}, headers)
	starredData := starred["data"].(map[string]any)
	if starredData["starred"] != true {
		t.Fatalf("expected starred draft, got %#v", starredData)
	}

	moved := requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/messages/"+draftID+"/move", map[string]any{
		"targetFolder": "archive",
	}, headers)
	movedData := moved["data"].(map[string]any)
	if movedData["folder"] != "archive" || movedData["previousFolder"] != "drafts" {
		t.Fatalf("expected moved draft, got %#v", movedData)
	}

	detail := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/messages/"+draftID, headers)
	message := detail["data"].(map[string]any)["message"].(map[string]any)
	if message["folder"] != "archive" || message["isStarred"] != true {
		t.Fatalf("expected detail to reflect actions, got %#v", message)
	}
}

func TestSendMessageCallsRemoteMailService(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotIdempotency string
	var gotPayload MailboxMessageRelayPayload
	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/messages/send" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		gotIdempotency = r.Header.Get("Idempotency-Key")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode message relay payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"providerMessageId":"provider-message-1","acceptedAt":"2026-05-25T00:00:00Z"}}`))
	}))
	t.Cleanup(mailServer.Close)

	store, session := newAuthenticatedMessageTestStore(t)
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:        "stalwart",
		Enabled:         true,
		BaseURL:         mailServer.URL,
		MessageSendPath: "/api/v1/messages/send",
		AdminToken:      "mail-admin-token",
		Status:          "online",
	}
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788", attachmentDir: t.TempDir()}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	headers := map[string]string{"Authorization": "Bearer " + session.Token}
	sent := requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/messages/send", map[string]any{
		"recipients": []string{"remote@example.com"},
		"subject":    "远端发信接口测试",
		"body": map[string]any{
			"format": "text",
			"text":   "通过真实邮件服务发信。",
		},
		"attachments": []any{map[string]any{
			"name":          "quote.pdf",
			"type":          "pdf",
			"contentType":   "application/pdf",
			"sizeBytes":     4,
			"contentBase64": "dGVzdA==",
		}},
		"source": "manual",
	}, headers)
	data := sent["data"].(map[string]any)
	if data["providerMessageId"] != "provider-message-1" {
		t.Fatalf("expected provider message id, got %#v", data)
	}
	message := data["message"].(map[string]any)
	if message["providerMessageId"] != "provider-message-1" || message["acceptedAt"] != "2026-05-25T00:00:00Z" {
		t.Fatalf("expected tracked provider fields on message, got %#v", message)
	}
	if gotPayload.AccountID != "account-message-1" || gotPayload.Email != "message-user@yuexiang.com" {
		t.Fatalf("unexpected relay account payload: %#v", gotPayload)
	}
	if len(gotPayload.Attachments) != 1 {
		t.Fatalf("expected relay attachment, got %#v", gotPayload.Attachments)
	}
	if len(gotPayload.Recipients) != 1 || gotPayload.Recipients[0] != "remote@example.com" {
		t.Fatalf("unexpected relay recipients: %#v", gotPayload)
	}
	if gotAuth != "Bearer mail-admin-token" {
		t.Fatalf("expected bearer auth header, got %q", gotAuth)
	}
	if gotIdempotency == "" {
		t.Fatalf("expected idempotency key")
	}
}

func TestSMTPOutboundSendsMessageWithoutHTTPSendEndpoint(t *testing.T) {
	t.Parallel()

	smtpAddr, messages := startFakeSMTPServer(t)
	host, portValue, err := net.SplitHostPort(smtpAddr)
	if err != nil {
		t.Fatalf("split smtp addr: %v", err)
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		t.Fatalf("parse smtp port: %v", err)
	}

	store, session := newAuthenticatedMessageTestStore(t)
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:        "stalwart",
		Enabled:         true,
		StrictDataPlane: true,
		SMTPEnabled:     true,
		SMTPHost:        host,
		SMTPPort:        port,
		SMTPTLSMode:     "none",
		Status:          "online",
	}
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788", attachmentDir: t.TempDir()}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	headers := map[string]string{"Authorization": "Bearer " + session.Token}
	sent := requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/messages/send", map[string]any{
		"recipients": []string{"smtp-target@example.com"},
		"subject":    "SMTP 原生投递测试",
		"body": map[string]any{
			"format": "text",
			"text":   "这封邮件通过 BFF SMTP 通道投递。",
		},
		"attachments": []any{},
		"source":      "manual",
	}, headers)
	message := sent["data"].(map[string]any)["message"].(map[string]any)
	if message["deliveryStatus"] != "accepted" || !strings.HasPrefix(message["providerMessageId"].(string), "smtp-") {
		t.Fatalf("expected SMTP accepted message, got %#v", message)
	}

	select {
	case raw := <-messages:
		if !strings.Contains(raw, "Subject: =?utf-8?q?SMTP_=E5=8E=9F=E7=94=9F=E6=8A=95=E9=80=92=E6=B5=8B=E8=AF=95?=") && !strings.Contains(raw, "Subject: SMTP 原生投递测试") {
			t.Fatalf("unexpected smtp subject: %s", raw)
		}
		if !strings.Contains(raw, "这封邮件通过 BFF SMTP 通道投递。") {
			t.Fatalf("unexpected smtp body: %s", raw)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("smtp server did not receive message")
	}
}

func TestSMTPEnvelopeRecipientsIncludesCcAndBcc(t *testing.T) {
	t.Parallel()

	recipients := smtpEnvelopeRecipients(SendMessagePayload{
		Recipients: []string{"to@example.com", "DUP@example.com"},
		CC:         []string{"cc@example.com", "dup@example.com"},
		BCC:        []string{"bcc@example.com"},
	})
	expected := []string{"to@example.com", "DUP@example.com", "cc@example.com", "bcc@example.com"}
	if !reflect.DeepEqual(recipients, expected) {
		t.Fatalf("unexpected smtp envelope recipients: %#v", recipients)
	}
}

func TestMailboxCredentialTemplatesUseProvisionedPassword(t *testing.T) {
	t.Parallel()

	secret, err := sealMailboxPassword("mailbox-secret")
	if err != nil {
		t.Fatalf("seal mailbox password: %v", err)
	}
	user := UserRecord{
		RegisteredUser: RegisteredUser{
			ID:    "account-credential-1",
			Email: "chenghong@yuexiang.com",
		},
		MailboxUsername:       "chenghong",
		MailboxPasswordSecret: secret,
	}

	imapUser, imapPassword, err := resolveIMAPCredentials(MailServerConfig{IMAPUsername: "{mailboxUsername}"}, user)
	if err != nil {
		t.Fatalf("resolve imap credentials: %v", err)
	}
	if imapUser != "chenghong" || imapPassword != "mailbox-secret" {
		t.Fatalf("unexpected imap credentials %q/%q", imapUser, imapPassword)
	}

	smtpUser, smtpPassword, shouldAuth, err := resolveSMTPCredentials(MailServerConfig{SMTPUsername: "{email}"}, user)
	if err != nil {
		t.Fatalf("resolve smtp credentials: %v", err)
	}
	if !shouldAuth || smtpUser != "chenghong@yuexiang.com" || smtpPassword != "mailbox-secret" {
		t.Fatalf("unexpected smtp credentials auth=%v user=%q password=%q", shouldAuth, smtpUser, smtpPassword)
	}
}

func TestRememberMailboxCredentialSealsPassword(t *testing.T) {
	t.Parallel()

	user := UserRecord{RegisteredUser: RegisteredUser{Email: "agent@yuexiang.com"}}
	err := rememberMailboxCredential(&user, MailboxProvisionResult{
		MailboxUsername: "agent@yuexiang.com",
		MailboxPassword: "generated-secret",
	})
	if err != nil {
		t.Fatalf("remember mailbox credential: %v", err)
	}
	if user.MailboxUsername != "agent@yuexiang.com" || user.MailboxPasswordSecret == "" {
		t.Fatalf("credential was not stored: %#v", user)
	}
	password, err := openMailboxPasswordSecret(user.MailboxPasswordSecret)
	if err != nil {
		t.Fatalf("open mailbox credential: %v", err)
	}
	if password != "generated-secret" {
		t.Fatalf("unexpected opened password: %q", password)
	}
}

func TestMailboxCredentialKeyEncryptsPassword(t *testing.T) {
	t.Setenv("MAILBOX_CREDENTIAL_KEY", "unit-test-mailbox-credential-key")

	secret, err := sealMailboxPassword("mailbox-secret")
	if err != nil {
		t.Fatalf("seal mailbox password: %v", err)
	}
	if strings.HasPrefix(secret, "plain:") {
		t.Fatalf("expected encrypted mailbox credential, got %q", secret)
	}
	password, err := openMailboxPasswordSecret(secret)
	if err != nil {
		t.Fatalf("open mailbox credential: %v", err)
	}
	if password != "mailbox-secret" {
		t.Fatalf("unexpected opened password: %q", password)
	}
}

func TestStrictModeRequiresMailboxCredentialKey(t *testing.T) {
	t.Setenv("INFINITEMAIL_PRODUCTION_STRICT", "true")
	t.Setenv("MAILBOX_CREDENTIAL_KEY", "")

	if _, err := sealMailboxPassword("mailbox-secret"); err == nil || !strings.Contains(err.Error(), "MAILBOX_CREDENTIAL_KEY") {
		t.Fatalf("expected strict mode to reject missing mailbox credential key, got %v", err)
	}

	status := buildDeploymentStatus(defaultAdminConfig(), "postgres")
	check, ok := deploymentCheckByID(status.Checks, "mailbox_credential_key")
	if !ok {
		t.Fatalf("missing mailbox credential key deployment check: %#v", status.Checks)
	}
	if check.Status != "blocking" || !check.Required {
		t.Fatalf("expected blocking mailbox credential key check, got %#v", check)
	}
}

func deploymentCheckByID(checks []DeploymentCheck, id string) (DeploymentCheck, bool) {
	for _, check := range checks {
		if check.ID == id {
			return check, true
		}
	}
	return DeploymentCheck{}, false
}

func TestListMessagesCallsRemoteMailServiceAndCachesItems(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotAccountID string
	var gotFolderID string
	var gotDetailAccountID string
	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/messages":
			gotAuth = r.Header.Get("Authorization")
			gotAccountID = r.URL.Query().Get("accountId")
			gotFolderID = r.URL.Query().Get("folderId")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"items":[{"id":"remote-inbox-1","folder":"inbox","sender":"Remote Sender","senderEmail":"remote@example.com","recipients":["message-user@yuexiang.com"],"subject":"远端收件测试","snippet":"来自真实邮件底座的收件。","sortAt":"2026-05-25T00:00:00Z","isUnread":true,"content":"<p>来自真实邮件底座的收件。</p>","attachments":[],"source":"mailbox","deliveryStatus":"received"}],"hasMore":false}}`))
		case "/api/v1/messages/remote-inbox-1":
			gotDetailAccountID = r.URL.Query().Get("accountId")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"message":{"id":"remote-inbox-1","folder":"inbox","sender":"Remote Sender","senderEmail":"remote@example.com","recipients":["message-user@yuexiang.com"],"subject":"远端收件测试","snippet":"来自真实邮件底座的详情。","sortAt":"2026-05-25T00:00:00Z","isUnread":true,"content":"<p>来自真实邮件底座的详情。</p>","attachments":[],"source":"mailbox","deliveryStatus":"received"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mailServer.Close)

	store, session := newAuthenticatedMessageTestStore(t)
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:          "stalwart",
		Enabled:           true,
		BaseURL:           mailServer.URL,
		MessageListPath:   "/api/v1/messages",
		MessageDetailPath: "/api/v1/messages/{messageId}",
		AdminToken:        "mail-admin-token",
		Status:            "online",
	}
	store.normalizeLocked()

	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	headers := map[string]string{"Authorization": "Bearer " + session.Token}
	list := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/messages?folderId=inbox", headers)
	data := list["data"].(map[string]any)
	items := data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one remote inbox item, got %#v", data)
	}
	message := items[0].(map[string]any)
	if message["id"] != "remote-inbox-1" || message["subject"] != "远端收件测试" {
		t.Fatalf("unexpected remote message: %#v", message)
	}
	counts := data["folderCounts"].(map[string]any)
	if counts["inbox"] != float64(1) {
		t.Fatalf("expected inbox unread count from cached remote item, got %#v", counts)
	}
	if gotAuth != "Bearer mail-admin-token" {
		t.Fatalf("expected bearer auth header, got %q", gotAuth)
	}
	if gotAccountID != "account-message-1" || gotFolderID != "inbox" {
		t.Fatalf("unexpected remote list query account=%q folder=%q", gotAccountID, gotFolderID)
	}

	detail := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/messages/remote-inbox-1", headers)
	detailMessage := detail["data"].(map[string]any)["message"].(map[string]any)
	if detailMessage["subject"] != "远端收件测试" {
		t.Fatalf("expected cached detail to load, got %#v", detailMessage)
	}
	if gotDetailAccountID != "account-message-1" {
		t.Fatalf("expected detail endpoint to be called with account id, got %q", gotDetailAccountID)
	}
}

func TestGetMessageDetailAlwaysUsesRemoteMailService(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotAccountID string
	detailCalls := 0
	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/messages/remote-detail-1" {
			http.NotFound(w, r)
			return
		}
		detailCalls += 1
		gotAuth = r.Header.Get("Authorization")
		gotAccountID = r.URL.Query().Get("accountId")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"message":{"id":"remote-detail-1","folder":"inbox","sender":"Detail Sender","senderEmail":"detail@example.com","recipients":["message-user@yuexiang.com"],"subject":"远端详情测试","snippet":"详情摘要","sortAt":"2026-05-25T01:00:00Z","isUnread":false,"content":"<p>完整详情正文</p>","attachments":[{"name":"contract.pdf","sizeBytes":1024}],"source":"mailbox","deliveryStatus":"received"}}}`))
	}))
	t.Cleanup(mailServer.Close)

	store, session := newAuthenticatedMessageTestStore(t)
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:          "stalwart",
		Enabled:           true,
		BaseURL:           mailServer.URL,
		MessageDetailPath: "/api/v1/messages/{messageId}",
		AdminToken:        "mail-admin-token",
		Status:            "online",
	}
	store.normalizeLocked()

	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	headers := map[string]string{"Authorization": "Bearer " + session.Token}
	detail := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/messages/remote-detail-1", headers)
	message := detail["data"].(map[string]any)["message"].(map[string]any)
	if message["subject"] != "远端详情测试" || message["hasAttachment"] != true {
		t.Fatalf("expected remote detail with attachment, got %#v", message)
	}
	if gotAuth != "Bearer mail-admin-token" || gotAccountID != "account-message-1" {
		t.Fatalf("unexpected remote detail auth/query auth=%q account=%q", gotAuth, gotAccountID)
	}

	cached := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/messages/remote-detail-1", headers)
	cachedMessage := cached["data"].(map[string]any)["message"].(map[string]any)
	if cachedMessage["content"] != "<p>完整详情正文</p>" {
		t.Fatalf("expected remote detail to stay consistent, got %#v", cachedMessage)
	}
	if detailCalls != 2 {
		t.Fatalf("expected every detail read to call the real detail endpoint, got %d calls", detailCalls)
	}
}

func TestMessageActionsSyncRemoteMailService(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotStarMethod string
	var gotMoveMethod string
	var gotReadMethod string
	var gotStarPayload MailboxMessageActionPayload
	var gotMovePayload MailboxMessageActionPayload
	var gotReadPayload MailboxMessageActionPayload
	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/messages/action-remote-1/star":
			gotAuth = r.Header.Get("Authorization")
			gotStarMethod = r.Method
			if err := json.NewDecoder(r.Body).Decode(&gotStarPayload); err != nil {
				t.Fatalf("decode star payload: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		case "/api/v1/messages/action-remote-1/move":
			gotMoveMethod = r.Method
			if err := json.NewDecoder(r.Body).Decode(&gotMovePayload); err != nil {
				t.Fatalf("decode move payload: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		case "/api/v1/messages/action-remote-1/read":
			gotReadMethod = r.Method
			if err := json.NewDecoder(r.Body).Decode(&gotReadPayload); err != nil {
				t.Fatalf("decode read payload: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mailServer.Close)

	store, session := newAuthenticatedMessageTestStore(t)
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:        "stalwart",
		Enabled:         true,
		BaseURL:         mailServer.URL,
		MessageStarPath: "/api/v1/messages/{messageId}/star",
		MessageMovePath: "/api/v1/messages/{messageId}/move",
		MessageReadPath: "/api/v1/messages/{messageId}/read",
		AdminToken:      "mail-admin-token",
		Status:          "online",
	}
	store.state.Messages["account-message-1"] = []MailMessage{{
		ID:             "action-remote-1",
		Folder:         "inbox",
		PreviousFolder: "inbox",
		Sender:         "Action Sender",
		SenderEmail:    "action@example.com",
		Recipients:     []string{"message-user@yuexiang.com"},
		Avatar:         "A",
		Role:           "邮件联系人",
		Subject:        "远端操作测试",
		Snippet:        "远端操作同步。",
		Time:           "刚刚",
		DateTimeLabel:  "2026年5月25日 16:00",
		SortAt:         "2026-05-25T16:00:00+08:00",
		IsUnread:       true,
		Tags:           []string{},
		Content:        "<p>远端操作同步。</p>",
		Attachments:    []any{},
		Source:         "mailbox",
		DeliveryStatus: "received",
	}}
	store.normalizeLocked()

	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	headers := map[string]string{"Authorization": "Bearer " + session.Token}
	starred := requestJSON[response](t, http.DefaultClient, http.MethodPatch, server.URL+"/api/v1/post-office/messages/action-remote-1/star", map[string]any{
		"starred": true,
	}, headers)
	if starred["data"].(map[string]any)["starred"] != true {
		t.Fatalf("expected local star result, got %#v", starred)
	}
	moved := requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/messages/action-remote-1/move", map[string]any{
		"targetFolder": "trash",
	}, headers)
	if moved["data"].(map[string]any)["folder"] != "trash" {
		t.Fatalf("expected local move result, got %#v", moved)
	}
	read := requestJSON[response](t, http.DefaultClient, http.MethodPatch, server.URL+"/api/v1/post-office/messages/action-remote-1/read", map[string]any{
		"isUnread": false,
	}, headers)
	if read["data"].(map[string]any)["isUnread"] != false {
		t.Fatalf("expected local read result, got %#v", read)
	}

	if gotAuth != "Bearer mail-admin-token" {
		t.Fatalf("expected bearer auth header, got %q", gotAuth)
	}
	if gotStarMethod != http.MethodPatch || gotMoveMethod != http.MethodPost || gotReadMethod != http.MethodPatch {
		t.Fatalf("unexpected remote methods star=%q move=%q read=%q", gotStarMethod, gotMoveMethod, gotReadMethod)
	}
	if gotStarPayload.AccountID != "account-message-1" || gotStarPayload.Starred == nil || *gotStarPayload.Starred != true {
		t.Fatalf("unexpected star payload: %#v", gotStarPayload)
	}
	if gotMovePayload.PreviousFolder != "inbox" || gotMovePayload.TargetFolder != "trash" {
		t.Fatalf("unexpected move payload: %#v", gotMovePayload)
	}
	if gotReadPayload.AccountID != "account-message-1" || gotReadPayload.IsUnread == nil || *gotReadPayload.IsUnread != false || gotReadPayload.Read == nil || *gotReadPayload.Read != true {
		t.Fatalf("unexpected read payload: %#v", gotReadPayload)
	}
}

func TestMessageReadStateUpdatesLocalUnreadCount(t *testing.T) {
	t.Parallel()

	remoteUnread := true
	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/messages":
			_ = json.NewEncoder(w).Encode(response{"data": response{"items": []response{{
				"id":             "read-local-1",
				"folder":         "inbox",
				"sender":         "Read Sender",
				"senderEmail":    "read@example.com",
				"recipients":     []string{"message-user@yuexiang.com"},
				"subject":        "已读状态测试",
				"snippet":        "打开邮件后未读数应减少。",
				"sortAt":         "2026-05-25T17:00:00+08:00",
				"isUnread":       remoteUnread,
				"content":        "<p>打开邮件后未读数应减少。</p>",
				"attachments":    []any{},
				"source":         "mailbox",
				"deliveryStatus": "received",
			}}, "hasMore": false}})
		case "/api/v1/messages/read-local-1/read":
			var payload MailboxMessageActionPayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode read payload: %v", err)
			}
			if payload.IsUnread != nil {
				remoteUnread = *payload.IsUnread
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mailServer.Close)

	store, session := newAuthenticatedMessageTestStore(t)
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:        "stalwart",
		Enabled:         true,
		BaseURL:         mailServer.URL,
		MessageListPath: "/api/v1/messages",
		MessageReadPath: "/api/v1/messages/{messageId}/read",
		Status:          "online",
	}
	store.state.Messages["account-message-1"] = []MailMessage{{
		ID:             "read-local-1",
		Folder:         "inbox",
		Sender:         "Read Sender",
		SenderEmail:    "read@example.com",
		Recipients:     []string{"message-user@yuexiang.com"},
		Avatar:         "R",
		Role:           "邮件联系人",
		Subject:        "已读状态测试",
		Snippet:        "打开邮件后未读数应减少。",
		Time:           "刚刚",
		DateTimeLabel:  "2026年5月25日 17:00",
		SortAt:         "2026-05-25T17:00:00+08:00",
		IsUnread:       true,
		Tags:           []string{},
		Content:        "<p>打开邮件后未读数应减少。</p>",
		Attachments:    []any{},
		Source:         "mailbox",
		DeliveryStatus: "received",
	}}
	store.normalizeLocked()

	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	headers := map[string]string{"Authorization": "Bearer " + session.Token}
	before := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/messages?folderId=inbox", headers)
	beforeCounts := before["data"].(map[string]any)["folderCounts"].(map[string]any)
	if beforeCounts["inbox"] != float64(1) {
		t.Fatalf("expected inbox unread count before read update, got %#v", beforeCounts)
	}

	updated := requestJSON[response](t, http.DefaultClient, http.MethodPatch, server.URL+"/api/v1/post-office/messages/read-local-1/read", map[string]any{
		"isUnread": false,
	}, headers)
	if updated["data"].(map[string]any)["read"] != true {
		t.Fatalf("expected read response, got %#v", updated)
	}

	after := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/messages?folderId=inbox", headers)
	afterData := after["data"].(map[string]any)
	afterCounts := afterData["folderCounts"].(map[string]any)
	if afterCounts["inbox"] != float64(0) {
		t.Fatalf("expected inbox unread count after read update, got %#v", afterCounts)
	}
	items := afterData["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["isUnread"] != false {
		t.Fatalf("expected message marked read, got %#v", items)
	}
}

func TestContactsAndThreadEndpointsUseServerMessages(t *testing.T) {
	t.Parallel()

	store, session := newAuthenticatedMessageTestStore(t)
	store.state.Messages["account-message-1"] = []MailMessage{{
		ID:             "contact-msg-1",
		Folder:         "inbox",
		Sender:         "客户一号",
		SenderEmail:    "client@example.com",
		Recipients:     []string{"message-user@yuexiang.com"},
		Avatar:         "客",
		Role:           "邮件联系人",
		Subject:        "联系人沉淀测试",
		Snippet:        "这封邮件应该进入通讯录。",
		Time:           "刚刚",
		DateTimeLabel:  "2026年5月25日 18:00",
		SortAt:         "2026-05-25T18:00:00+08:00",
		IsUnread:       true,
		Tags:           []string{"真实邮件"},
		Content:        "<p>这封邮件应该进入通讯录。</p>",
		Attachments:    []any{},
		Source:         "mailbox",
		DeliveryStatus: "received",
	}}
	store.normalizeLocked()

	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	headers := map[string]string{"Authorization": "Bearer " + session.Token}
	contacts := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/contacts", headers)
	items := contacts["data"].(map[string]any)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one contact, got %#v", contacts)
	}
	contact := items[0].(map[string]any)
	if contact["email"] != "client@example.com" || contact["name"] != "客户一号" {
		t.Fatalf("unexpected contact: %#v", contact)
	}
	contactID := contact["id"].(string)
	thread := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/contacts/"+contactID+"/thread", headers)
	threadData := thread["data"].(map[string]any)
	threadItems := threadData["items"].([]any)
	if len(threadItems) != 1 || threadItems[0].(map[string]any)["id"] != "contact-msg-1" {
		t.Fatalf("expected contact thread item, got %#v", threadData)
	}
}

func TestTemplateSendEndpointCreatesRealOutgoingMessage(t *testing.T) {
	t.Parallel()

	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/messages/send":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"providerMessageId":"provider-template-1","acceptedAt":"2026-05-25T03:00:00Z"}}`))
		case "/api/v1/messages":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"items":[],"hasMore":false}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mailServer.Close)

	store, session := newAuthenticatedMessageTestStore(t)
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:        "stalwart",
		Enabled:         true,
		BaseURL:         mailServer.URL,
		MessageSendPath: "/api/v1/messages/send",
		MessageListPath: "/api/v1/messages",
		Status:          "online",
	}
	store.normalizeLocked()

	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	headers := map[string]string{"Authorization": "Bearer " + session.Token}
	templates := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/templates", headers)
	templateItems := templates["data"].(map[string]any)["items"].([]any)
	if len(templateItems) == 0 {
		t.Fatalf("expected backend templates, got %#v", templates)
	}

	sent := requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/templates/send", map[string]any{
		"role":       "account",
		"recipients": []string{"target@example.com"},
	}, headers)
	data := sent["data"].(map[string]any)
	if data["recipientCount"] != float64(1) || data["role"] != "account" {
		t.Fatalf("unexpected template send response: %#v", data)
	}
	message := data["message"].(map[string]any)
	if message["source"] != "template" || message["folder"] != "sent" {
		t.Fatalf("expected outgoing template message, got %#v", message)
	}

	list := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/messages?folderId=sent", headers)
	items := list["data"].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["source"] != "template" {
		t.Fatalf("expected sent template message to be listed, got %#v", list)
	}
}

func TestInboundMailWebhookPersistsReceivedMessage(t *testing.T) {
	t.Parallel()

	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/messages" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"items":[],"hasMore":false}}`))
	}))
	t.Cleanup(mailServer.Close)

	store, session := newAuthenticatedMessageTestStore(t)
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:        "stalwart",
		Enabled:         true,
		BaseURL:         mailServer.URL,
		MessageListPath: "/api/v1/messages",
		Status:          "online",
	}
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788", mailWebhookToken: "hook-secret"}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	webhookHeaders := map[string]string{"X-Mail-Webhook-Token": "hook-secret"}
	ingested := requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/webhooks/mail/inbound", map[string]any{
		"from":      "客户二号 <client-two@example.com>",
		"to":        []string{"message-user@yuexiang.com"},
		"subject":   "真实收信 Webhook",
		"html":      "<p>这是一封来自邮件底座的真实收信。</p>",
		"messageId": "provider-inbound-1",
	}, webhookHeaders)
	message := ingested["data"].(map[string]any)["message"].(map[string]any)
	if message["folder"] != "inbox" || message["senderEmail"] != "client-two@example.com" {
		t.Fatalf("unexpected inbound message: %#v", message)
	}

	userHeaders := map[string]string{"Authorization": "Bearer " + session.Token}
	list := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/messages?folderId=inbox", userHeaders)
	items := list["data"].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["subject"] != "真实收信 Webhook" {
		t.Fatalf("expected inbound message in inbox, got %#v", list)
	}
}

func TestSettingsPatchKeepsAutoReplyEnabled(t *testing.T) {
	t.Parallel()

	store, session := newAuthenticatedMessageTestStore(t)
	store.state.Settings["account-message-1"] = MailSettings{
		DefaultSenderName: "Message User",
		Signature:         "old",
		AutoReplyEnabled:  true,
		AutoReplyMessage:  "我稍后回复。",
		UpdatedAt:         time.Now().Add(-time.Hour).Format(time.RFC3339),
	}
	store.normalizeLocked()

	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	headers := map[string]string{"Authorization": "Bearer " + session.Token}
	updated := requestJSON[response](t, http.DefaultClient, http.MethodPut, server.URL+"/api/v1/post-office/settings", map[string]any{
		"signature": "new signature",
	}, headers)
	settings := updated["data"].(map[string]any)["settings"].(map[string]any)
	if settings["autoReplyEnabled"] != true || settings["autoReplyMessage"] != "我稍后回复。" {
		t.Fatalf("partial settings update should preserve auto reply fields, got %#v", settings)
	}
	if settings["signature"] != "new signature" {
		t.Fatalf("expected updated signature, got %#v", settings)
	}
}

func TestInboundMailWebhookSendsAutoReply(t *testing.T) {
	t.Parallel()

	smtpAddr, messages := startFakeSMTPServer(t)
	host, portValue, err := net.SplitHostPort(smtpAddr)
	if err != nil {
		t.Fatalf("split smtp addr: %v", err)
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		t.Fatalf("parse smtp port: %v", err)
	}

	store, _ := newAuthenticatedMessageTestStore(t)
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:        "stalwart",
		Enabled:         true,
		StrictDataPlane: true,
		SMTPEnabled:     true,
		SMTPHost:        host,
		SMTPPort:        port,
		SMTPTLSMode:     "none",
		Status:          "online",
	}
	store.state.Settings["account-message-1"] = MailSettings{
		DefaultSenderName: "Message User",
		Signature:         "",
		AutoReplyEnabled:  true,
		AutoReplyMessage:  "我正在处理其他事项，稍后会回复。",
		UpdatedAt:         time.Now().Format(time.RFC3339),
	}
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788", mailWebhookToken: "hook-secret", attachmentDir: t.TempDir()}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	webhookHeaders := map[string]string{"X-Mail-Webhook-Token": "hook-secret"}
	_ = requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/webhooks/mail/inbound", map[string]any{
		"from":      "客户三号 <client-three@example.com>",
		"to":        []string{"message-user@yuexiang.com"},
		"subject":   "需要报价",
		"text":      "请尽快给我一份报价。",
		"messageId": "provider-inbound-auto-1",
	}, webhookHeaders)

	select {
	case raw := <-messages:
		if !strings.Contains(raw, "client-three@example.com") {
			t.Fatalf("auto reply should target original sender, got %s", raw)
		}
		if !strings.Contains(raw, "Subject: =?utf-8?q?Re:_=E9=9C=80=E8=A6=81=E6=8A=A5=E4=BB=B7?=") && !strings.Contains(raw, "Subject: Re: 需要报价") {
			t.Fatalf("unexpected auto reply subject: %s", raw)
		}
		if !strings.Contains(raw, "我正在处理其他事项，稍后会回复。") {
			t.Fatalf("unexpected auto reply body: %s", raw)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("smtp server did not receive auto reply")
	}

	snapshot := store.snapshot()
	autoReplyCount := 0
	for _, message := range snapshot.Messages["account-message-1"] {
		if message.Source == "auto_reply" && message.ThreadID == "provider-inbound-auto-1" {
			autoReplyCount += 1
			if message.DeliveryStatus != "accepted" || message.ProviderMessageID == "" {
				t.Fatalf("expected accepted auto reply with provider id, got %#v", message)
			}
		}
	}
	if autoReplyCount != 1 {
		t.Fatalf("expected exactly one stored auto reply, got %d in %#v", autoReplyCount, snapshot.Messages["account-message-1"])
	}

	_ = requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/webhooks/mail/inbound", map[string]any{
		"from":      "客户三号 <client-three@example.com>",
		"to":        []string{"message-user@yuexiang.com"},
		"subject":   "需要报价",
		"text":      "重复投递。",
		"messageId": "provider-inbound-auto-1",
	}, webhookHeaders)
	snapshot = store.snapshot()
	autoReplyCount = 0
	for _, message := range snapshot.Messages["account-message-1"] {
		if message.Source == "auto_reply" && message.ThreadID == "provider-inbound-auto-1" {
			autoReplyCount += 1
		}
	}
	if autoReplyCount != 1 {
		t.Fatalf("duplicate inbound webhook should not send another auto reply, got %d", autoReplyCount)
	}
}

func TestDeliveryWebhookUpdatesStoredMessage(t *testing.T) {
	t.Parallel()

	store, _ := newAuthenticatedMessageTestStore(t)
	store.state.Messages["account-message-1"] = []MailMessage{{
		ID:                "sent-delivery-1",
		Folder:            "sent",
		PreviousFolder:    "sent",
		Sender:            "Message User",
		SenderEmail:       "message-user@yuexiang.com",
		Recipients:        []string{"target@example.com"},
		Subject:           "投递状态测试",
		Snippet:           "投递状态测试",
		Time:              "刚刚",
		DateTimeLabel:     "2026年5月25日 19:00",
		SortAt:            "2026-05-25T19:00:00+08:00",
		SentAt:            "2026-05-25T19:00:00+08:00",
		Tags:              []string{"已发送"},
		IsOutgoing:        true,
		Content:           "<p>投递状态测试</p>",
		Attachments:       []any{},
		Source:            "mailbox",
		DeliveryStatus:    "accepted",
		ProviderMessageID: "provider-delivery-1",
	}}
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788", mailWebhookToken: "hook-secret"}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	updated := requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/webhooks/mail/delivery", map[string]any{
		"providerMessageId": "provider-delivery-1",
		"status":            "failed",
		"deliveryError":     "mailbox unavailable",
	}, map[string]string{"X-Mail-Webhook-Token": "hook-secret"})
	message := updated["data"].(map[string]any)["message"].(map[string]any)
	if message["deliveryStatus"] != "failed" || message["deliveryError"] != "mailbox unavailable" {
		t.Fatalf("expected failed delivery status, got %#v", message)
	}
}

func TestListMessagesHealthCheckBaseURLRejectsLocalFallback(t *testing.T) {
	t.Parallel()

	called := false
	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(mailServer.Close)

	store, session := newAuthenticatedMessageTestStore(t)
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider: "stalwart",
		Enabled:  true,
		BaseURL:  mailServer.URL + "/healthz",
		Status:   "online",
	}
	store.state.Messages["account-message-1"] = []MailMessage{{
		ID:             "local-inbox-1",
		Folder:         "inbox",
		PreviousFolder: "inbox",
		Sender:         "Local Sender",
		SenderEmail:    "local@example.com",
		Recipients:     []string{"message-user@yuexiang.com"},
		Avatar:         "L",
		Role:           "邮件联系人",
		Subject:        "缓存收件",
		Snippet:        "缺少真实列表接口时不能直接返回缓存。",
		Time:           "刚刚",
		DateTimeLabel:  "2026年5月25日 16:00",
		SortAt:         "2026-05-25T16:00:00+08:00",
		ReceivedAt:     "2026-05-25T16:00:00+08:00",
		IsUnread:       true,
		Tags:           []string{},
		Content:        "<p>缺少真实列表接口时不能直接返回缓存。</p>",
		Attachments:    []any{},
		Source:         "mailbox",
		DeliveryStatus: "received",
	}}
	store.normalizeLocked()

	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	headers := map[string]string{"Authorization": "Bearer " + session.Token}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/post-office/messages?folderId=inbox", nil)
	if err != nil {
		t.Fatalf("new list request: %v", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected missing message list endpoint to reject cached fallback, got %d", res.StatusCode)
	}
	if called {
		t.Fatalf("health check URL should not be used as a message list endpoint")
	}
}

func TestIMAPDataPlaneListsDetailsActionsAndDrafts(t *testing.T) {
	t.Parallel()

	imapAddr, appended := startFakeIMAPServer(t)
	host, portRaw, err := net.SplitHostPort(imapAddr)
	if err != nil {
		t.Fatalf("split imap addr: %v", err)
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		t.Fatalf("parse imap port: %v", err)
	}

	store, session := newAuthenticatedMessageTestStore(t)
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:        "imap",
		StrictDataPlane: true,
		IMAPEnabled:     true,
		IMAPHost:        host,
		IMAPPort:        port,
		IMAPUsername:    "{email}",
		IMAPPassword:    "secret",
		IMAPTLSMode:     "none",
		Status:          "online",
	}
	store.normalizeLocked()
	if missing := mailServerDataPlaneMissing(store.state.Config.Mailbox.Server); len(missing) != 1 || missing[0] != "发信接口或 SMTP 发信" {
		t.Fatalf("expected IMAP to satisfy receive/detail/action/draft data plane, got %#v", missing)
	}

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788", attachmentDir: t.TempDir()}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	headers := map[string]string{"Authorization": "Bearer " + session.Token}
	list := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/messages?folderId=inbox", headers)
	items := list["data"].(map[string]any)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one IMAP message, got %#v", list)
	}
	item := items[0].(map[string]any)
	if item["source"] != "imap" || item["subject"] != "IMAP 真实收件" || item["isUnread"] != true {
		t.Fatalf("unexpected IMAP list item: %#v", item)
	}
	messageID := item["id"].(string)
	if !strings.HasPrefix(messageID, "imap-") {
		t.Fatalf("expected encoded IMAP message id, got %s", messageID)
	}

	detail := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/messages/"+messageID, headers)
	message := detail["data"].(map[string]any)["message"].(map[string]any)
	if !strings.Contains(message["content"].(string), "这是一封来自 IMAP 的真实邮件") {
		t.Fatalf("expected IMAP detail content, got %#v", message)
	}

	starred := requestJSON[response](t, http.DefaultClient, http.MethodPatch, server.URL+"/api/v1/post-office/messages/"+messageID+"/star", map[string]any{
		"starred": true,
	}, headers)
	if starred["data"].(map[string]any)["starred"] != true {
		t.Fatalf("expected IMAP star response, got %#v", starred)
	}
	read := requestJSON[response](t, http.DefaultClient, http.MethodPatch, server.URL+"/api/v1/post-office/messages/"+messageID+"/read", map[string]any{
		"isUnread": false,
	}, headers)
	if read["data"].(map[string]any)["read"] != true {
		t.Fatalf("expected IMAP read response, got %#v", read)
	}

	draft := requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/drafts", map[string]any{
		"recipients": []string{"target@example.com"},
		"subject":    "IMAP 草稿",
		"body": map[string]any{
			"format": "text",
			"text":   "草稿内容",
		},
	}, headers)
	if draft["data"].(map[string]any)["draft"].(map[string]any)["deliveryStatus"] != "draft" {
		t.Fatalf("expected IMAP draft response, got %#v", draft)
	}
	select {
	case raw := <-appended:
		if !strings.Contains(raw, "草稿内容") {
			t.Fatalf("expected appended draft MIME, got %s", raw)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected fake IMAP server to receive appended draft")
	}
}

func TestStrictDataPlaneRejectsHealthCheckBaseURLLocalCompatibility(t *testing.T) {
	t.Parallel()

	called := false
	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(mailServer.Close)

	store, session := newAuthenticatedMessageTestStore(t)
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:        "stalwart",
		Enabled:         true,
		StrictDataPlane: true,
		BaseURL:         mailServer.URL + "/healthz",
		Status:          "online",
	}
	store.state.Messages["account-message-1"] = []MailMessage{{
		ID:             "local-inbox-1",
		Folder:         "inbox",
		Sender:         "Local Sender",
		SenderEmail:    "local@example.com",
		Recipients:     []string{"message-user@yuexiang.com"},
		Subject:        "缓存收件",
		SortAt:         "2026-05-25T16:00:00+08:00",
		IsUnread:       true,
		Tags:           []string{},
		Attachments:    []any{},
		Source:         "mailbox",
		DeliveryStatus: "received",
	}}
	store.normalizeLocked()

	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/post-office/messages?folderId=inbox", nil)
	if err != nil {
		t.Fatalf("new strict list request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+session.Token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("strict list request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected strict data plane to reject cached fallback, got %d", res.StatusCode)
	}
	if called {
		t.Fatalf("strict data plane should fail before calling health check URL as a list endpoint")
	}
}

func TestOAuthStartReturnsRedirectWhenProviderConfigured(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	store.state.Config.Auth.OAuthClientID = "client-id"
	store.state.Config.Auth.OAuthClientSecret = "client-secret"
	store.state.Config.Auth.OAuthAuthorizeURL = "https://id.example.test/oauth/authorize"
	store.state.Config.Auth.OAuthTokenURL = "https://id.example.test/oauth/token"
	store.state.Config.Auth.OAuthUserInfoURL = "https://id.example.test/oauth/userinfo"
	store.state.Config.Auth.OAuthRedirectURL = "http://127.0.0.1:1666/api/v1/post-office/auth/oauth/callback"
	store.normalizeLocked()

	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	res := requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/auth/oauth/start", map[string]any{}, nil)
	redirectURL, ok := res["redirectUrl"].(string)
	if !ok || !strings.Contains(redirectURL, "client_id=client-id") || !strings.Contains(redirectURL, "response_type=code") {
		t.Fatalf("expected OAuth redirect URL, got %#v", res)
	}
	if res["isAuthenticated"] == true {
		t.Fatalf("configured OAuth start should only return a redirect, got session payload: %#v", res)
	}
}

func TestOAuthStartRejectsDevelopmentFallbackByDefault(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	store.normalizeLocked()

	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/post-office/auth/oauth/start", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("new oauth request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("oauth start request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected missing OAuth config to reject dev fallback, got %d", res.StatusCode)
	}
}

func TestAliyunSMSRequiresCompleteConfigWhenEnabled(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	store.state.Config.SMS.AliyunEnabled = true
	store.state.Config.SMS.AccessKeyID = "access-key-id"
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788"}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	status := postJSONStatus(t, http.DefaultClient, server.URL+"/api/v1/post-office/auth/sms/send", map[string]any{
		"phone":   "13800138999",
		"purpose": "login",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected incomplete Aliyun SMS config to fail, got %d", status)
	}
	if len(store.snapshot().Config.SMSLogs) != 0 {
		t.Fatalf("failed Aliyun SMS send should not create a usable code log")
	}
}

func TestSMSRequiresAliyunOrExplicitLocalDebug(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788"}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	status := postJSONStatus(t, http.DefaultClient, server.URL+"/api/v1/post-office/auth/sms/send", map[string]any{
		"phone":   "13800138998",
		"purpose": "login",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected SMS without Aliyun or debug switch to fail, got %d", status)
	}
	if len(store.snapshot().Config.SMSLogs) != 0 {
		t.Fatalf("rejected SMS send should not create code logs")
	}
}

func TestCompanyModeAllowsMultipleCompanyUserInvites(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788"}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	client := http.DefaultClient
	_ = postJSON[InviteRecord](t, client, server.URL+"/api/v1/post-office/admin/mail/invites", map[string]any{
		"prefix":      "user",
		"emailPrefix": "company-one",
	})

	secondStatus := postJSONStatus(t, client, server.URL+"/api/v1/post-office/admin/mail/invites", map[string]any{
		"prefix":      "user",
		"emailPrefix": "company-two",
	})
	if secondStatus != http.StatusOK {
		t.Fatalf("expected second company invite to be allowed, got %d", secondStatus)
	}
}

func TestCompanyModeAllowsOpenRegistrationForMultipleCompanyUsers(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	client := http.DefaultClient
	configStatus := requestJSONStatus(t, client, http.MethodPatch, server.URL+"/api/v1/post-office/admin/mail/config", map[string]any{
		"auth": map[string]any{
			"inviteRequired": false,
		},
	})
	if configStatus != http.StatusOK {
		t.Fatalf("expected config update 200, got %d", configStatus)
	}

	firstSMS := postJSON[SMSLogRecord](t, client, server.URL+"/api/v1/post-office/auth/sms/send", map[string]any{
		"phone":   "13800138101",
		"purpose": "register",
	})
	_ = postJSON[response](t, client, server.URL+"/api/v1/post-office/auth/register", map[string]any{
		"phone":       "13800138101",
		"smsCode":     firstSMS.Code,
		"password":    "Passw0rd",
		"displayName": "Company One",
		"emailPrefix": "company-open-one",
		"prefix":      "user",
	})

	secondSMS := postJSON[SMSLogRecord](t, client, server.URL+"/api/v1/post-office/auth/sms/send", map[string]any{
		"phone":   "13800138102",
		"purpose": "register",
	})
	secondStatus := postJSONStatus(t, client, server.URL+"/api/v1/post-office/auth/register", map[string]any{
		"phone":       "13800138102",
		"smsCode":     secondSMS.Code,
		"password":    "Passw0rd",
		"displayName": "Company Two",
		"emailPrefix": "company-open-two",
		"prefix":      "user",
	})
	if secondStatus != http.StatusOK {
		t.Fatalf("expected second open registration to be allowed, got %d", secondStatus)
	}
}

func TestCompanyModeAllowsRegistrationWithoutStorageQuota(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	store.state.Config.Auth.InviteRequired = false
	store.state.Messages["existing"] = []MailMessage{{
		ID:      "oversized",
		Folder:  "inbox",
		Subject: "oversized attachment",
		Attachments: []any{
			map[string]any{"name": "archive.zip", "sizeBytes": int64(1024 * 1024 * 1024)},
		},
	}}
	store.normalizeLocked()

	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	client := http.DefaultClient
	sms := postJSON[SMSLogRecord](t, client, server.URL+"/api/v1/post-office/auth/sms/send", map[string]any{
		"phone":   "13800138111",
		"purpose": "register",
	})
	status := postJSONStatus(t, client, server.URL+"/api/v1/post-office/auth/register", map[string]any{
		"phone":       "13800138111",
		"smsCode":     sms.Code,
		"password":    "Passw0rd",
		"displayName": "Quota User",
		"emailPrefix": "quota",
		"prefix":      "user",
	})
	if status != http.StatusOK {
		t.Fatalf("expected registration to work with unmetered storage, got %d", status)
	}

	config := getJSON[response](t, client, server.URL+"/api/v1/post-office/admin/mail/config")
	usage := config["usage"].(map[string]any)
	if usage["storageUsedMb"] == float64(0) || usage["storageLimitGb"] != float64(0) {
		t.Fatalf("expected unmetered storage usage snapshot, got %#v", usage)
	}
}

func TestOperationalTasksProvisionQueuedMailbox(t *testing.T) {
	t.Parallel()

	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/mailboxes":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"externalId":"principal-provision-1","status":"provisioned"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mailServer.Close)

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := time.Now().Add(-time.Minute).Format(time.RFC3339)
	user := UserRecord{
		RegisteredUser: RegisteredUser{
			ID:            "account-provision-1",
			Phone:         "13800138222",
			Email:         "user-provision@yuexiang.com",
			DisplayName:   "Provision User",
			RegisteredAt:  now,
			Source:        "invite",
			Status:        "active",
			MailboxStatus: "queued",
		},
		PasswordHash: "argon2id:test",
	}
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:      "stalwart",
		Enabled:       true,
		BaseURL:       mailServer.URL + "/healthz",
		ProvisionPath: mailServer.URL + "/api/v1/mailboxes",
		Status:        "online",
	}
	store.state.Users = []UserRecord{user}
	store.state.Config.RegisteredUsers = []RegisteredUser{user.RegisteredUser}
	store.state.Config.ProvisionJobs = []ProvisionJob{{
		ID:        "prov-test-1",
		AccountID: user.ID,
		Email:     user.Email,
		Status:    "queued",
		NextRunAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}}
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788"}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	result := postJSON[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/admin/mail/ops/run", map[string]any{})
	summary := result["summary"].(map[string]any)
	provisioning := summary["provisioning"].(map[string]any)
	if provisioning["succeeded"] != float64(1) {
		t.Fatalf("expected one succeeded provision job, got %#v", provisioning)
	}
	config := result["config"].(map[string]any)
	jobs := config["provisionJobs"].([]any)
	if len(jobs) != 1 || jobs[0].(map[string]any)["status"] != "succeeded" {
		t.Fatalf("expected succeeded provision job in config, got %#v", jobs)
	}
	users := config["registeredUsers"].([]any)
	if users[0].(map[string]any)["mailboxStatus"] != "provisioned" {
		t.Fatalf("expected provisioned account, got %#v", users[0])
	}
	ops := config["ops"].(map[string]any)
	if ops["lastRunStatus"] != "success" || ops["lastRunAt"] == "" {
		t.Fatalf("expected ops last run metadata, got %#v", ops)
	}
}

func TestRetryMailboxProvisionRunsRealProvisionImmediately(t *testing.T) {
	t.Parallel()

	called := false
	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/mailboxes" {
			http.NotFound(w, r)
			return
		}
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"externalId":"principal-retry-1","status":"provisioned"}`))
	}))
	t.Cleanup(mailServer.Close)

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := time.Now().Add(-time.Minute).Format(time.RFC3339)
	user := UserRecord{
		RegisteredUser: RegisteredUser{
			ID:            "account-retry-1",
			Phone:         "13800138226",
			Email:         "retry-provision@yuexiang.com",
			DisplayName:   "Retry User",
			RegisteredAt:  now,
			Source:        "invite",
			Status:        "active",
			MailboxStatus: "failed",
		},
		PasswordHash: "argon2id:test",
	}
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:      "stalwart",
		Enabled:       true,
		BaseURL:       mailServer.URL,
		ProvisionPath: "/api/v1/mailboxes",
		Status:        "online",
	}
	store.state.Users = []UserRecord{user}
	store.state.Config.RegisteredUsers = []RegisteredUser{user.RegisteredUser}
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788"}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	result := postJSON[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/admin/mail/accounts/account-retry-1/provision", map[string]any{})
	job := result["job"].(map[string]any)
	if !called || job["status"] != "succeeded" {
		t.Fatalf("expected immediate real provision success, called=%t result=%#v", called, result)
	}
	account := result["account"].(map[string]any)
	if account["mailboxStatus"] != "provisioned" || account["mailboxExternalId"] != "principal-retry-1" {
		t.Fatalf("expected provisioned account, got %#v", account)
	}
}

func TestRetryMailboxProvisionRejectsMissingMailServer(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := time.Now().Add(-time.Minute).Format(time.RFC3339)
	user := UserRecord{
		RegisteredUser: RegisteredUser{
			ID:            "account-retry-missing-1",
			Phone:         "13800138227",
			Email:         "retry-missing@yuexiang.com",
			DisplayName:   "Retry Missing",
			RegisteredAt:  now,
			Source:        "invite",
			Status:        "active",
			MailboxStatus: "failed",
		},
		PasswordHash: "argon2id:test",
	}
	store.state.Users = []UserRecord{user}
	store.state.Config.RegisteredUsers = []RegisteredUser{user.RegisteredUser}
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788"}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	status := postJSONStatus(t, http.DefaultClient, server.URL+"/api/v1/post-office/admin/mail/accounts/account-retry-missing-1/provision", map[string]any{})
	if status != http.StatusBadRequest {
		t.Fatalf("expected missing mail server retry to fail, got %d", status)
	}
	snapshot := store.snapshot()
	if len(snapshot.Config.ProvisionJobs) != 1 || snapshot.Config.ProvisionJobs[0].Status != "blocked" {
		t.Fatalf("expected blocked provision job to be saved, got %#v", snapshot.Config.ProvisionJobs)
	}
	if snapshot.Config.RegisteredUsers[0].MailboxStatus != "pending_config" {
		t.Fatalf("expected account pending config, got %#v", snapshot.Config.RegisteredUsers[0])
	}
}

func TestOpsConfigPatch(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788"}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	config := patchJSON[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/admin/mail/config", map[string]any{
		"ops": map[string]any{
			"autoRunEnabled":  true,
			"intervalMinutes": 2,
		},
	})
	ops := config["ops"].(map[string]any)
	if ops["autoRunEnabled"] != true || ops["intervalMinutes"] != float64(2) {
		t.Fatalf("expected saved ops config, got %#v", ops)
	}

	result := postJSON[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/admin/mail/ops/run", map[string]any{})
	nextConfig := result["config"].(map[string]any)
	nextOps := nextConfig["ops"].(map[string]any)
	if nextOps["lastRunStatus"] != "success" || !strings.Contains(nextOps["lastRunMessage"].(string), "开通队列暂无待处理任务") {
		t.Fatalf("expected successful last run metadata, got %#v", nextOps)
	}
}

func TestOperationalTasksCallsMailProvisionEndpoint(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotIdempotency string
	var gotPayload MailboxProvisionPayload
	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/mailboxes":
			gotAuth = r.Header.Get("Authorization")
			gotIdempotency = r.Header.Get("Idempotency-Key")
			if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
				t.Fatalf("decode provision payload: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"externalId":"principal-user-remote","status":"provisioned"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mailServer.Close)

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := time.Now().Add(-time.Minute).Format(time.RFC3339)
	user := UserRecord{
		RegisteredUser: RegisteredUser{
			ID:            "account-remote-1",
			Phone:         "13800138223",
			Email:         "user-remote@yuexiang.com",
			DisplayName:   "Remote User",
			RegisteredAt:  now,
			Source:        "invite",
			Status:        "active",
			MailboxStatus: "queued",
		},
		PasswordHash: "argon2id:test",
	}
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:      "stalwart",
		Enabled:       true,
		BaseURL:       mailServer.URL + "/healthz",
		ProvisionPath: mailServer.URL + "/api/v1/mailboxes",
		AdminToken:    "mail-admin-token",
		Status:        "online",
	}
	store.state.Users = []UserRecord{user}
	store.state.Config.RegisteredUsers = []RegisteredUser{user.RegisteredUser}
	store.state.Config.ProvisionJobs = []ProvisionJob{{
		ID:        "prov-remote-1",
		AccountID: user.ID,
		Email:     user.Email,
		Status:    "queued",
		NextRunAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}}
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788"}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	result := postJSON[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/admin/mail/ops/run", map[string]any{})
	config := result["config"].(map[string]any)
	users := config["registeredUsers"].([]any)
	if users[0].(map[string]any)["mailboxExternalId"] != "principal-user-remote" {
		t.Fatalf("expected remote external id, got %#v", users[0])
	}
	if gotPayload.Email != "user-remote@yuexiang.com" || gotPayload.LocalPart != "user-remote" || gotPayload.Domain != "yuexiang.com" {
		t.Fatalf("unexpected provision payload: %#v", gotPayload)
	}
	if gotPayload.Password == "" || gotPayload.QuotaBytes != 0 {
		t.Fatalf("expected password and unmetered quota in payload: %#v", gotPayload)
	}
	if gotAuth != "Bearer mail-admin-token" {
		t.Fatalf("expected bearer auth header, got %q", gotAuth)
	}
	if gotIdempotency != "prov-remote-1" {
		t.Fatalf("expected idempotency key, got %q", gotIdempotency)
	}
	mailbox := config["mailbox"].(map[string]any)
	mailServerConfig := mailbox["server"].(map[string]any)
	if mailServerConfig["adminToken"] != nil {
		t.Fatalf("admin token leaked in response: %#v", mailServerConfig)
	}
	if mailServerConfig["adminTokenSet"] != true {
		t.Fatalf("expected redacted token marker, got %#v", mailServerConfig)
	}
}

func TestOperationalTasksProvisionStalwartJMAPAccountWithoutCustomProvisionPath(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var seenMethods []string
	var gotDomainCreate map[string]any
	var gotAccountCreate map[string]any
	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		var envelope map[string]any
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode jmap payload: %v", err)
		}
		methodCalls := anySliceFromAny(envelope["methodCalls"])
		if len(methodCalls) != 1 {
			t.Fatalf("expected one JMAP method call, got %#v", envelope)
		}
		call := anySliceFromAny(methodCalls[0])
		if len(call) < 3 {
			t.Fatalf("unexpected method call: %#v", call)
		}
		methodName := stringFromAny(call[0])
		clientID := stringFromAny(call[2])
		seenMethods = append(seenMethods, methodName)
		args := mapFromAny(call[1])
		w.Header().Set("Content-Type", "application/json")
		switch methodName {
		case "x:Domain/query":
			_, _ = w.Write([]byte(fmt.Sprintf(`{"methodResponses":[["x:Domain/query",{"ids":[]},"%s"]]}`, clientID)))
		case "x:Domain/set":
			create := mapFromAny(args["create"])
			gotDomainCreate = mapFromAny(create["domain"])
			_, _ = w.Write([]byte(fmt.Sprintf(`{"methodResponses":[["x:Domain/set",{"created":{"domain":{"id":"domain-created-id"}}},"%s"]]}`, clientID)))
		case "x:Account/query":
			filter := mapFromAny(args["filter"])
			if filter["name"] != "jmap-user" || filter["domainId"] != "domain-created-id" {
				t.Fatalf("unexpected account query filter: %#v", filter)
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"methodResponses":[["x:Account/query",{"ids":[]},"%s"]]}`, clientID)))
		case "x:Account/set":
			create := mapFromAny(args["create"])
			gotAccountCreate = mapFromAny(create["account"])
			_, _ = w.Write([]byte(fmt.Sprintf(`{"methodResponses":[["x:Account/set",{"created":{"account":{"id":"account-created-id"}}},"%s"]]}`, clientID)))
		default:
			t.Fatalf("unexpected JMAP method: %s", methodName)
		}
	}))
	t.Cleanup(mailServer.Close)

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := time.Now().Add(-time.Minute).Format(time.RFC3339)
	user := UserRecord{
		RegisteredUser: RegisteredUser{
			ID:            "account-jmap-1",
			Phone:         "13800138225",
			Email:         "jmap-user@yuexiang.com",
			DisplayName:   "JMAP User",
			RegisteredAt:  now,
			Source:        "invite",
			Status:        "active",
			MailboxStatus: "queued",
		},
		PasswordHash: "argon2id:test",
	}
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:   "stalwart",
		Enabled:    true,
		BaseURL:    mailServer.URL,
		AdminToken: "mail-admin-token",
		Status:     "online",
	}
	store.state.Users = []UserRecord{user}
	store.state.Config.RegisteredUsers = []RegisteredUser{user.RegisteredUser}
	store.state.Config.ProvisionJobs = []ProvisionJob{{
		ID:        "prov-jmap-1",
		AccountID: user.ID,
		Email:     user.Email,
		Status:    "queued",
		NextRunAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}}
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788"}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	result := postJSON[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/admin/mail/ops/run", map[string]any{})
	config := result["config"].(map[string]any)
	users := config["registeredUsers"].([]any)
	if users[0].(map[string]any)["mailboxExternalId"] != "account-created-id" {
		t.Fatalf("expected Stalwart external id, got %#v", users[0])
	}
	if strings.Join(seenMethods, ",") != "x:Domain/query,x:Domain/set,x:Account/query,x:Account/set" {
		t.Fatalf("unexpected JMAP methods: %#v", seenMethods)
	}
	if gotAuth != "Bearer mail-admin-token" {
		t.Fatalf("expected bearer auth header, got %q", gotAuth)
	}
	if gotDomainCreate["name"] != "yuexiang.com" {
		t.Fatalf("unexpected domain create payload: %#v", gotDomainCreate)
	}
	if gotAccountCreate["name"] != "jmap-user" || gotAccountCreate["domainId"] != "domain-created-id" {
		t.Fatalf("unexpected account create payload: %#v", gotAccountCreate)
	}
	credentials := mapFromAny(gotAccountCreate["credentials"])
	password := mapFromAny(credentials["password"])
	if password["@type"] != "Password" || stringFromAny(password["secret"]) == "" {
		t.Fatalf("expected generated account password credential, got %#v", credentials)
	}
	mailbox := config["mailbox"].(map[string]any)
	mailServerConfig := mailbox["server"].(map[string]any)
	if mailServerConfig["adminToken"] != nil {
		t.Fatalf("admin token leaked in response: %#v", mailServerConfig)
	}
	if mailServerConfig["adminTokenSet"] != true {
		t.Fatalf("expected redacted token marker, got %#v", mailServerConfig)
	}
}

func TestAccountLifecycleSyncsStalwartJMAPWithoutCustomLifecyclePath(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotUpdate map[string]any
	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		var envelope map[string]any
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode jmap payload: %v", err)
		}
		methodCalls := anySliceFromAny(envelope["methodCalls"])
		call := anySliceFromAny(methodCalls[0])
		methodName := stringFromAny(call[0])
		clientID := stringFromAny(call[2])
		args := mapFromAny(call[1])
		w.Header().Set("Content-Type", "application/json")
		switch methodName {
		case "x:Account/set":
			update := mapFromAny(args["update"])
			gotUpdate = mapFromAny(update["principal-life-1"])
			_, _ = w.Write([]byte(fmt.Sprintf(`{"methodResponses":[["x:Account/set",{"updated":{"principal-life-1":null}},"%s"]]}`, clientID)))
		default:
			t.Fatalf("unexpected JMAP method: %s", methodName)
		}
	}))
	t.Cleanup(mailServer.Close)

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := time.Now().Add(-time.Minute).Format(time.RFC3339)
	user := UserRecord{
		RegisteredUser: RegisteredUser{
			ID:                "account-life-1",
			Phone:             "13800138224",
			Email:             "user-life@yuexiang.com",
			DisplayName:       "Life User",
			RegisteredAt:      now,
			Source:            "invite",
			Status:            "active",
			MailboxStatus:     "provisioned",
			MailboxExternalID: "principal-life-1",
		},
		PasswordHash: "argon2id:test",
	}
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:   "stalwart",
		Enabled:    true,
		BaseURL:    mailServer.URL,
		AdminToken: "mail-admin-token",
		Status:     "online",
	}
	store.state.Users = []UserRecord{user}
	store.state.Config.RegisteredUsers = []RegisteredUser{user.RegisteredUser}
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788"}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	disabled := postJSON[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/admin/mail/accounts/account-life-1/disable", map[string]any{})
	if disabled["account"].(map[string]any)["status"] != "disabled" {
		t.Fatalf("expected disabled account, got %#v", disabled)
	}
	if gotAuth != "Bearer mail-admin-token" {
		t.Fatalf("expected bearer auth header, got %q", gotAuth)
	}
	permissions := mapFromAny(gotUpdate["permissions"])
	if permissions["@type"] != "Replace" || len(stringSliceFromAny(permissions["disabledPermissions"])) == 0 {
		t.Fatalf("expected disabled permissions update, got %#v", gotUpdate)
	}
	config := getJSON[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/admin/mail/config")
	mailbox := config["mailbox"].(map[string]any)
	mailServerConfig := mailbox["server"].(map[string]any)
	if mailServerConfig["lastLifecycleSyncAt"] == "" {
		t.Fatalf("expected lifecycle sync timestamp, got %#v", mailServerConfig)
	}
}

func TestAccountLifecycleSyncsMailServer(t *testing.T) {
	t.Parallel()

	var actions []string
	var resetPassword string
	var gotAuth string
	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/mailboxes/lifecycle":
			gotAuth = r.Header.Get("Authorization")
			var payload MailboxLifecyclePayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode lifecycle payload: %v", err)
			}
			actions = append(actions, payload.Action)
			if payload.Action == "reset_password" {
				resetPassword = payload.Password
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"externalId":"principal-life-1","status":"synced"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mailServer.Close)

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := time.Now().Add(-time.Minute).Format(time.RFC3339)
	user := UserRecord{
		RegisteredUser: RegisteredUser{
			ID:                "account-life-1",
			Phone:             "13800138224",
			Email:             "user-life@yuexiang.com",
			DisplayName:       "Life User",
			RegisteredAt:      now,
			Source:            "invite",
			Status:            "active",
			MailboxStatus:     "provisioned",
			MailboxExternalID: "principal-life-1",
		},
		PasswordHash: "argon2id:test",
	}
	store.state.Config.Mailbox.Server = MailServerConfig{
		Provider:      "stalwart",
		Enabled:       true,
		BaseURL:       mailServer.URL + "/healthz",
		LifecyclePath: mailServer.URL + "/api/v1/mailboxes/lifecycle",
		AdminToken:    "mail-admin-token",
		Status:        "online",
	}
	store.state.Users = []UserRecord{user}
	store.state.Config.RegisteredUsers = []RegisteredUser{user.RegisteredUser}
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788"}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	disabled := postJSON[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/admin/mail/accounts/account-life-1/disable", map[string]any{})
	if disabled["account"].(map[string]any)["status"] != "disabled" {
		t.Fatalf("expected disabled account, got %#v", disabled)
	}
	enabled := postJSON[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/admin/mail/accounts/account-life-1/enable", map[string]any{})
	if enabled["account"].(map[string]any)["status"] != "active" {
		t.Fatalf("expected enabled account, got %#v", enabled)
	}
	reset := postJSON[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/admin/mail/accounts/account-life-1/reset-password", map[string]any{})
	if reset["temporaryPassword"] == "" || resetPassword != reset["temporaryPassword"] {
		t.Fatalf("expected reset password to be synced, response=%#v synced=%q", reset, resetPassword)
	}
	if strings.Join(actions, ",") != "disable,enable,reset_password" {
		t.Fatalf("unexpected lifecycle actions: %#v", actions)
	}
	if gotAuth != "Bearer mail-admin-token" {
		t.Fatalf("expected bearer auth header, got %q", gotAuth)
	}
	config := getJSON[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/admin/mail/config")
	mailbox := config["mailbox"].(map[string]any)
	mailServerConfig := mailbox["server"].(map[string]any)
	if mailServerConfig["adminToken"] != nil {
		t.Fatalf("admin token leaked in config: %#v", mailServerConfig)
	}
	if mailServerConfig["lastLifecycleSyncAt"] == "" {
		t.Fatalf("expected lifecycle sync timestamp, got %#v", mailServerConfig)
	}
}

func TestSMSCodeThrottle(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	client := http.DefaultClient
	_ = postJSON[SMSLogRecord](t, client, server.URL+"/api/v1/post-office/auth/sms/send", map[string]any{
		"phone":   "13800138008",
		"purpose": "register",
	})

	raw, err := json.Marshal(map[string]any{
		"phone":   "13800138008",
		"purpose": "register",
	})
	if err != nil {
		t.Fatalf("marshal second sms payload: %v", err)
	}
	res, err := http.Post(server.URL+"/api/v1/post-office/auth/sms/send", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("second sms request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected throttled second sms request, got %d", res.StatusCode)
	}
}

func TestSMSCodeStoresHashAndRedactsSecrets(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	app := localSMSTestApp(&App{store: store, userAppOrigin: "http://127.0.0.1:1788"})
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	sms := postJSON[SMSLogRecord](t, http.DefaultClient, server.URL+"/api/v1/post-office/auth/sms/send", map[string]any{
		"phone":   "13800138018",
		"purpose": "login",
	})
	if sms.Code == "" {
		t.Fatalf("local sms response should keep debug code: %#v", sms)
	}
	if sms.CodeHash != "" {
		t.Fatalf("sms response leaked code hash: %#v", sms)
	}

	state := store.snapshot()
	if len(state.Config.SMSLogs) != 1 {
		t.Fatalf("expected one sms log, got %#v", state.Config.SMSLogs)
	}
	stored := state.Config.SMSLogs[0]
	if stored.CodeHash == "" || stored.CodeMasked == "" {
		t.Fatalf("expected stored sms hash and mask, got %#v", stored)
	}
	if err := ensureSMSCode(state.Config.SMSLogs, "13800138018", sms.Code, "login"); err != nil {
		t.Fatalf("hashed sms code should verify: %v", err)
	}
	session := getJSON[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/auth/session")
	publicConfig := session["adminConfig"].(map[string]any)
	if logs, ok := publicConfig["smsLogs"].([]any); !ok || len(logs) != 0 {
		t.Fatalf("public session config should not expose sms logs: %#v", publicConfig["smsLogs"])
	}

	redacted := redactSMSLog(SMSLogRecord{
		ID:         "sms_aliyun",
		Phone:      "13800138018",
		Code:       "123456",
		CodeHash:   stored.CodeHash,
		CodeMasked: "12****",
		Purpose:    "login",
		Provider:   "aliyun",
		Status:     "sent",
		CreatedAt:  nowISO(),
		ExpiresAt:  time.Now().Add(time.Minute).Format(time.RFC3339),
	})
	if redacted.Code != "" || redacted.CodeHash != "" || redacted.CodeMasked != "12****" {
		t.Fatalf("aliyun sms log should be redacted, got %#v", redacted)
	}
}

func TestPasswordHashUsesArgon2idAndVerifiesLegacyHash(t *testing.T) {
	hash, err := hashPassword("Passw0rd")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if !strings.HasPrefix(hash, "argon2id:") {
		t.Fatalf("expected argon2id hash, got %q", hash)
	}
	if !verifyPassword(hash, "Passw0rd") {
		t.Fatalf("argon2id password did not verify")
	}
	if verifyPassword(hash, "wrong-password") {
		t.Fatalf("argon2id password verified with wrong password")
	}

	legacyDigest := sha256.Sum256([]byte("salt:legacy-pass"))
	legacyHash := "sha256:salt:" + hex.EncodeToString(legacyDigest[:])
	if !verifyPassword(legacyHash, "legacy-pass") {
		t.Fatalf("legacy sha256 hash should still verify during migration")
	}
}

func TestRecommendedDNSRecords(t *testing.T) {
	records := recommendedDNSRecords("Example.COM")
	if len(records) != 4 {
		t.Fatalf("expected four dns records, got %#v", records)
	}
	if records[0].Type != "MX" || records[0].Expected != "10 mail.example.com" {
		t.Fatalf("unexpected mx recommendation: %#v", records[0])
	}
	if records[2].Host != "_dmarc" || !strings.Contains(records[2].Expected, "dmarc@example.com") {
		t.Fatalf("unexpected dmarc recommendation: %#v", records[2])
	}
}

func TestMailboxProvisionState(t *testing.T) {
	user := UserRecord{RegisteredUser: RegisteredUser{ID: "account-1", Email: "user-one@example.com"}}
	applyMailboxProvisionState(&user, defaultMailServerConfig())
	if user.MailboxStatus != "pending_config" {
		t.Fatalf("expected pending_config without mail server, got %q", user.MailboxStatus)
	}

	applyMailboxProvisionState(&user, MailServerConfig{
		Provider: "stalwart",
		Enabled:  true,
		BaseURL:  "http://127.0.0.1:8080",
		Status:   "online",
	})
	if user.MailboxStatus != "queued" {
		t.Fatalf("expected queued with online mail server, got %q", user.MailboxStatus)
	}
}

func TestSessionRequiresProvisionedMailboxBeforeWorkspace(t *testing.T) {
	session := SessionRecord{Token: "token-1", UserID: "account-1"}
	user := UserRecord{RegisteredUser: RegisteredUser{
		ID:            "account-1",
		Email:         "user-one@example.com",
		DisplayName:   "User One",
		RegisteredAt:  nowISO(),
		Status:        "active",
		MailboxStatus: "queued",
	}}
	payload := sessionPayload("token-1", session, user, defaultAdminConfig())
	if payload["requiresActivation"] != false || payload["requiresProvisioning"] != true {
		t.Fatalf("expected queued mailbox to require provisioning only, got %#v", payload)
	}
	if payload["mailboxProvisioned"] != false || payload["provisioningStatus"] != "queued" {
		t.Fatalf("expected mailbox not provisioned, got %#v", payload)
	}
	profile := payload["profile"].(MailProfile)
	if profile.MailboxProvisioned || profile.ProvisionedAt != "" {
		t.Fatalf("expected profile to block workspace until real provision, got %#v", profile)
	}
}

func TestMailboxAPIsRejectUnprovisionedAccount(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := nowISO()
	user := UserRecord{RegisteredUser: RegisteredUser{
		ID:            "account-queued-1",
		Phone:         "13800138888",
		Email:         "user-queued@yuexiang.com",
		DisplayName:   "Queued User",
		RegisteredAt:  now,
		Source:        "test",
		Status:        "active",
		MailboxStatus: "queued",
	}}
	session := createSession(user.ID)
	store.state.Users = []UserRecord{user}
	store.state.Config.RegisteredUsers = []RegisteredUser{user.RegisteredUser}
	putSession(store.state.Sessions, session)
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788"}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/post-office/messages?folderId=inbox", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+session.Token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request messages: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected unprovisioned mailbox to be blocked, got %d", res.StatusCode)
	}
}

func TestSessionStorageUsesHashedTokenKeys(t *testing.T) {
	sessions := map[string]SessionRecord{}
	session := SessionRecord{
		Token:     "bff_super_secret_token",
		UserID:    "account-1",
		CreatedAt: time.Now().Format(time.RFC3339),
		ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339),
	}
	putSession(sessions, session)

	if _, ok := sessions[session.Token]; ok {
		t.Fatalf("raw session token should not be used as the storage key")
	}
	key := sessionStorageKey(session.Token)
	if !isSessionStorageKey(key) {
		t.Fatalf("expected hashed session storage key, got %q", key)
	}
	current, ok := lookupSession(sessions, session.Token)
	if !ok || current.UserID != session.UserID {
		t.Fatalf("hashed session lookup failed: %#v", current)
	}
}

func TestSecuritySessionsListAndLogoutOthers(t *testing.T) {
	t.Parallel()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	user := UserRecord{
		RegisteredUser: RegisteredUser{
			ID:           "account-security-1",
			Phone:        "13800138999",
			Email:        "security-user@yuexiang.com",
			DisplayName:  "Security User",
			RegisteredAt: time.Now().Add(-time.Hour).Format(time.RFC3339),
			Source:       "test",
			Status:       "active",
		},
		PasswordHash: "argon2id:test",
	}
	currentSession := createSession(user.ID)
	otherSession := createSession(user.ID)
	otherSession.IP = "10.0.0.2"
	otherSession.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120 Safari/537.36"
	otherSession.Device = deviceLabelFromUserAgent(otherSession.UserAgent)
	store.state.Users = []UserRecord{user}
	store.state.Config.RegisteredUsers = []RegisteredUser{user.RegisteredUser}
	putSession(store.state.Sessions, currentSession)
	putSession(store.state.Sessions, otherSession)
	store.normalizeLocked()

	app := &App{store: store, userAppOrigin: "http://127.0.0.1:1788", attachmentDir: t.TempDir()}
	mux := http.NewServeMux()
	app.routes(mux)
	server := httptest.NewServer(withCORS(mux))
	t.Cleanup(server.Close)

	headers := map[string]string{
		"Authorization":   "Bearer " + currentSession.Token,
		"User-Agent":      "Feishu/7.2 Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
		"X-Forwarded-For": "203.0.113.9",
	}
	listed := getJSONWithHeaders[response](t, http.DefaultClient, server.URL+"/api/v1/post-office/security/sessions", headers)
	listData := listed["data"].(map[string]any)
	items := listData["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 security sessions, got %#v", listed)
	}
	if items[0].(map[string]any)["current"] != true {
		t.Fatalf("expected current session to be first after touch, got %#v", items)
	}
	if got := store.snapshot().Sessions[sessionStorageKey(currentSession.Token)].IP; got != "203.0.113.9" {
		t.Fatalf("expected current session IP to be updated, got %q", got)
	}

	loggedOut := requestJSON[response](t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/post-office/security/sessions/logout-others", map[string]any{}, headers)
	logoutData := loggedOut["data"].(map[string]any)
	if int(logoutData["removed"].(float64)) != 1 {
		t.Fatalf("expected one removed session, got %#v", loggedOut)
	}
	if _, ok := lookupSession(store.snapshot().Sessions, otherSession.Token); ok {
		t.Fatalf("other session should have been removed")
	}
	if _, ok := lookupSession(store.snapshot().Sessions, currentSession.Token); !ok {
		t.Fatalf("current session should remain valid")
	}
}

func TestCORSAllowsAdminTokenHeader(t *testing.T) {
	handler := withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/post-office/admin/mail/config", nil)
	req.Header.Set("Origin", "http://127.0.0.1:1888")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for preflight, got %d", res.Code)
	}
	allowedHeaders := res.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowedHeaders, "X-Admin-Token") {
		t.Fatalf("admin token header is missing from CORS allow list: %q", allowedHeaders)
	}
}

func postJSON[T any](t *testing.T, client *http.Client, url string, payload any) T {
	t.Helper()

	return requestJSON[T](t, client, http.MethodPost, url, payload, nil)
}

func patchJSON[T any](t *testing.T, client *http.Client, url string, payload any) T {
	t.Helper()

	return requestJSON[T](t, client, http.MethodPatch, url, payload, nil)
}

func requestJSON[T any](t *testing.T, client *http.Client, method string, url string, payload any, headers map[string]string) T {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		t.Fatalf("%s %s status %d", method, url, res.StatusCode)
	}

	var out T
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func postJSONStatus(t *testing.T, client *http.Client, url string, payload any) int {
	t.Helper()
	return requestJSONStatus(t, client, http.MethodPost, url, payload)
}

func requestJSONStatus(t *testing.T, client *http.Client, method string, url string, payload any) int {
	t.Helper()
	return requestJSONStatusWithHeaders(t, client, method, url, payload, nil)
}

func requestJSONStatusWithHeaders(t *testing.T, client *http.Client, method string, url string, payload any, headers map[string]string) int {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal %s payload: %v", method, err)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("new %s request: %v", method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	return res.StatusCode
}

func newAuthenticatedMessageTestStore(t *testing.T) (*Store, SessionRecord) {
	t.Helper()

	store, err := newStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := time.Now().Add(-time.Minute).Format(time.RFC3339)
	user := UserRecord{
		RegisteredUser: RegisteredUser{
			ID:                   "account-message-1",
			Phone:                "13800138333",
			Email:                "message-user@yuexiang.com",
			DisplayName:          "Message User",
			RegisteredAt:         now,
			Source:               "test",
			Status:               "active",
			MailboxStatus:        "provisioned",
			MailboxProvisionedAt: now,
			MailboxExternalID:    "principal-message-1",
		},
		PasswordHash: "argon2id:test",
	}
	session := createSession(user.ID)
	store.state.Users = []UserRecord{user}
	store.state.Config.RegisteredUsers = []RegisteredUser{user.RegisteredUser}
	putSession(store.state.Sessions, session)
	store.normalizeLocked()
	return store, session
}

func getJSONWithHeaders[T any](t *testing.T, client *http.Client, url string, headers map[string]string) T {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new get request: %v", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		t.Fatalf("get %s status %d", url, res.StatusCode)
	}

	var out T
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func getJSON[T any](t *testing.T, client *http.Client, url string) T {
	t.Helper()

	res, err := client.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		t.Fatalf("get %s status %d", url, res.StatusCode)
	}

	var out T
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func auditItemsContain(items []any, action string) bool {
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if record["action"] == action {
			return true
		}
	}
	return false
}

func startFakeSMTPServer(t *testing.T) (string, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen smtp: %v", err)
	}
	messages := make(chan string, 2)
	t.Cleanup(func() {
		_ = listener.Close()
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleFakeSMTPConn(conn, messages)
		}
	}()
	return listener.Addr().String(), messages
}

func handleFakeSMTPConn(conn net.Conn, messages chan<- string) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	writeSMTPLine(writer, "220 localhost ESMTP")
	var data strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
			writeSMTPLine(writer, "250 localhost")
		case strings.HasPrefix(command, "MAIL FROM:"):
			writeSMTPLine(writer, "250 OK")
		case strings.HasPrefix(command, "RCPT TO:"):
			writeSMTPLine(writer, "250 OK")
		case command == "DATA":
			writeSMTPLine(writer, "354 End data with <CR><LF>.<CR><LF>")
			data.Reset()
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimSpace(dataLine) == "." {
					break
				}
				data.WriteString(dataLine)
			}
			messages <- data.String()
			writeSMTPLine(writer, "250 queued")
		case command == "QUIT":
			writeSMTPLine(writer, "221 bye")
			return
		default:
			writeSMTPLine(writer, "250 OK")
		}
	}
}

func writeSMTPLine(writer *bufio.Writer, value string) {
	_, _ = writer.WriteString(value + "\r\n")
	_ = writer.Flush()
}

type fakeIMAPMessage struct {
	UID   uint32
	Raw   string
	Flags map[string]bool
}

func startFakeIMAPServer(t *testing.T) (string, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen imap: %v", err)
	}
	appended := make(chan string, 2)
	messages := map[string][]*fakeIMAPMessage{
		"INBOX": {
			{
				UID: 101,
				Raw: strings.Join([]string{
					"From: IMAP Client <client@example.com>",
					"To: message-user@yuexiang.com",
					"Subject: IMAP 真实收件",
					"Date: Mon, 25 May 2026 10:30:00 +0800",
					"Message-ID: <imap-101@example.com>",
					"MIME-Version: 1.0",
					"Content-Type: text/plain; charset=utf-8",
					"Content-Transfer-Encoding: 8bit",
					"",
					"这是一封来自 IMAP 的真实邮件。",
					"",
				}, "\r\n"),
				Flags: map[string]bool{},
			},
		},
		"Drafts": {},
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleFakeIMAPConn(conn, messages, appended)
		}
	}()
	return listener.Addr().String(), appended
}

func handleFakeIMAPConn(conn net.Conn, messages map[string][]*fakeIMAPMessage, appended chan<- string) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	writeIMAPLine(writer, "* OK fake IMAP ready")
	selected := "INBOX"
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		tag := fields[0]
		upper := strings.ToUpper(line)
		switch {
		case strings.Contains(upper, " CAPABILITY"):
			writeIMAPLine(writer, "* CAPABILITY IMAP4rev1 MOVE")
			writeIMAPLine(writer, tag+" OK CAPABILITY completed")
		case strings.Contains(upper, " LOGIN "):
			writeIMAPLine(writer, tag+" OK LOGIN completed")
		case strings.Contains(upper, " SELECT "):
			selected = fakeIMAPMailboxFromCommand(line)
			if _, ok := messages[selected]; !ok {
				writeIMAPLine(writer, tag+" NO mailbox missing")
				continue
			}
			writeIMAPLine(writer, "* "+strconv.Itoa(len(messages[selected]))+" EXISTS")
			writeIMAPLine(writer, tag+" OK SELECT completed")
		case strings.Contains(upper, " UID SEARCH "):
			criteria := "ALL"
			if strings.Contains(upper, "UNSEEN") {
				criteria = "UNSEEN"
			} else if strings.Contains(upper, "FLAGGED") {
				criteria = "FLAGGED"
			}
			uids := []string{}
			for _, message := range messages[selected] {
				if fakeIMAPMatchesSearch(message, criteria) {
					uids = append(uids, strconv.FormatUint(uint64(message.UID), 10))
				}
			}
			writeIMAPLine(writer, "* SEARCH "+strings.Join(uids, " "))
			writeIMAPLine(writer, tag+" OK SEARCH completed")
		case strings.Contains(upper, " UID FETCH "):
			uidSet := fakeIMAPUIDSetFromFetch(line)
			sequence := 1
			for _, message := range messages[selected] {
				if !fakeIMAPUIDWanted(message.UID, uidSet) {
					continue
				}
				raw := []byte(message.Raw)
				writeIMAPLine(writer, fmt.Sprintf("* %d FETCH (UID %d FLAGS (%s) INTERNALDATE \"25-May-2026 10:30:00 +0800\" RFC822.SIZE %d BODY[] {%d}", sequence, message.UID, fakeIMAPFlags(message), len(raw), len(raw)))
				_, _ = writer.Write(raw)
				_, _ = writer.WriteString("\r\n)\r\n")
				_ = writer.Flush()
				sequence += 1
			}
			writeIMAPLine(writer, tag+" OK FETCH completed")
		case strings.Contains(upper, " UID STORE "):
			uid := fakeIMAPUIDFromStore(line)
			for _, message := range messages[selected] {
				if message.UID != uid {
					continue
				}
				add := strings.Contains(upper, "+FLAGS")
				if strings.Contains(upper, `\FLAGGED`) {
					message.Flags[`\Flagged`] = add
				}
				if strings.Contains(upper, `\SEEN`) {
					message.Flags[`\Seen`] = add
				}
			}
			writeIMAPLine(writer, tag+" OK STORE completed")
		case strings.Contains(upper, " APPEND "):
			size, ok := imapLiteralSize(line)
			if !ok {
				writeIMAPLine(writer, tag+" BAD missing literal")
				continue
			}
			writeIMAPLine(writer, "+ go ahead")
			raw := make([]byte, size)
			if _, err := io.ReadFull(reader, raw); err != nil {
				return
			}
			_, _ = reader.ReadString('\n')
			appended <- string(raw)
			messages["Drafts"] = append(messages["Drafts"], &fakeIMAPMessage{
				UID:   uint32(200 + len(messages["Drafts"])),
				Raw:   string(raw),
				Flags: map[string]bool{`\Draft`: true},
			})
			writeIMAPLine(writer, tag+" OK APPEND completed")
		case strings.Contains(upper, " LOGOUT"):
			writeIMAPLine(writer, "* BYE logging out")
			writeIMAPLine(writer, tag+" OK LOGOUT completed")
			return
		default:
			writeIMAPLine(writer, tag+" OK completed")
		}
	}
}

func writeIMAPLine(writer *bufio.Writer, value string) {
	_, _ = writer.WriteString(value + "\r\n")
	_ = writer.Flush()
}

func fakeIMAPMailboxFromCommand(line string) string {
	start := strings.Index(line, `"`)
	end := strings.LastIndex(line, `"`)
	if start >= 0 && end > start {
		return line[start+1 : end]
	}
	fields := strings.Fields(line)
	if len(fields) > 2 {
		return strings.Trim(fields[len(fields)-1], `"`)
	}
	return "INBOX"
}

func fakeIMAPMatchesSearch(message *fakeIMAPMessage, criteria string) bool {
	switch criteria {
	case "UNSEEN":
		return !message.Flags[`\Seen`]
	case "FLAGGED":
		return message.Flags[`\Flagged`]
	default:
		return !message.Flags[`\Deleted`]
	}
}

func fakeIMAPFlags(message *fakeIMAPMessage) string {
	flags := []string{}
	for flag, enabled := range message.Flags {
		if enabled {
			flags = append(flags, flag)
		}
	}
	return strings.Join(flags, " ")
}

func fakeIMAPUIDSetFromFetch(line string) string {
	upper := strings.ToUpper(line)
	index := strings.Index(upper, " UID FETCH ")
	if index < 0 {
		return ""
	}
	rest := strings.TrimSpace(line[index+len(" UID FETCH "):])
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func fakeIMAPUIDWanted(uid uint32, uidSet string) bool {
	for _, item := range strings.Split(uidSet, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, ":") {
			parts := strings.SplitN(item, ":", 2)
			start, _ := strconv.ParseUint(parts[0], 10, 32)
			end, _ := strconv.ParseUint(parts[1], 10, 32)
			if uint64(uid) >= start && uint64(uid) <= end {
				return true
			}
			continue
		}
		parsed, err := strconv.ParseUint(item, 10, 32)
		if err == nil && uint32(parsed) == uid {
			return true
		}
	}
	return false
}

func fakeIMAPUIDFromStore(line string) uint32 {
	upper := strings.ToUpper(line)
	index := strings.Index(upper, " UID STORE ")
	if index < 0 {
		return 0
	}
	rest := strings.TrimSpace(line[index+len(" UID STORE "):])
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return 0
	}
	uid, _ := strconv.ParseUint(fields[0], 10, 32)
	return uint32(uid)
}
