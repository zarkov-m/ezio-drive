package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	graphBaseURL = "https://graph.microsoft.com/v1.0"
)

var scopes = []string{"User.Read", "Mail.Read", "Mail.Send", "offline_access"}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

type simpleCache struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"` // unix seconds
	TokenType    string `json:"token_type,omitempty"`
}

// Legacy MSAL-like cache blobs written by the Python script.
type legacyAccessToken struct {
	Secret    string `json:"secret"`
	ExpiresOn string `json:"expires_on"`
}

type legacyRefreshToken struct {
	Secret string `json:"secret"`
}

type legacyCache struct {
	AccessToken  map[string]legacyAccessToken  `json:"AccessToken"`
	RefreshToken map[string]legacyRefreshToken `json:"RefreshToken"`
}

type graphRecipient struct {
	EmailAddress struct {
		Address string `json:"address"`
		Name    string `json:"name,omitempty"`
	} `json:"emailAddress"`
}

type graphSendPayload struct {
	Message struct {
		Subject string `json:"subject"`
		Body    struct {
			ContentType string `json:"contentType"`
			Content     string `json:"content"`
		} `json:"body"`
		ToRecipients  []graphRecipient `json:"toRecipients"`
		CcRecipients  []graphRecipient `json:"ccRecipients"`
		BccRecipients []graphRecipient `json:"bccRecipients"`
		Attachments   []map[string]any `json:"attachments,omitempty"`
	} `json:"message"`
	SaveToSentItems bool `json:"saveToSentItems"`
}

type graphMessage struct {
	ID                string           `json:"id"`
	Subject           string           `json:"subject"`
	ReceivedDateTime  string           `json:"receivedDateTime"`
	BodyPreview       string           `json:"bodyPreview"`
	HasAttachments    bool             `json:"hasAttachments"`
	IsRead            bool             `json:"isRead"`
	ConversationID    string           `json:"conversationId"`
	InternetMessageID string           `json:"internetMessageId"`
	From              graphRecipient   `json:"from"`
	ToRecipients      []graphRecipient `json:"toRecipients"`
	CcRecipients      []graphRecipient `json:"ccRecipients"`
	Body              struct {
		ContentType string `json:"contentType"`
		Content     string `json:"content"`
	} `json:"body"`
}

type graphMessageList struct {
	Value []graphMessage `json:"value"`
}

type graphAttachment struct {
	ODataType    string `json:"@odata.type"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	ContentType  string `json:"contentType"`
	Size         int64  `json:"size"`
	IsInline     bool   `json:"isInline"`
	ContentBytes string `json:"contentBytes"`
}

type graphAttachmentList struct {
	Value []graphAttachment `json:"value"`
}

type graphMe struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	UserPrincipalName string `json:"userPrincipalName"`
	Mail              string `json:"mail"`
}

func env(name string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		fatalf("Missing required env var: %s", name)
	}
	return v
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}

func expandHome(path string) string {
	if path == "" {
		return path
	}
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func parseRecipients(value string) []string {
	value = strings.ReplaceAll(value, ";", ",")
	chunks := strings.Split(value, ",")
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		addr := strings.TrimSpace(c)
		if addr != "" {
			out = append(out, addr)
		}
	}
	return out
}

func makeRecipients(addrs []string) []graphRecipient {
	out := make([]graphRecipient, 0, len(addrs))
	for _, a := range addrs {
		var r graphRecipient
		r.EmailAddress.Address = a
		out = append(out, r)
	}
	return out
}

func loadCache(path string) (simpleCache, error) {
	var out simpleCache
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return out, err
	}

	// 1) Try our native cache format first.
	if err := json.Unmarshal(raw, &out); err == nil && out.AccessToken != "" {
		return out, nil
	}

	// 2) Fallback: try legacy MSAL cache format.
	var legacy legacyCache
	if err := json.Unmarshal(raw, &legacy); err == nil {
		for _, at := range legacy.AccessToken {
			out.AccessToken = at.Secret
			if n, err := strconv.ParseInt(strings.TrimSpace(at.ExpiresOn), 10, 64); err == nil {
				out.ExpiresAt = n
			}
			break
		}
		for _, rt := range legacy.RefreshToken {
			out.RefreshToken = rt.Secret
			break
		}
		if out.AccessToken != "" || out.RefreshToken != "" {
			return out, nil
		}
	}

	return out, nil
}

func saveCache(path string, c simpleCache) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	blob, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, blob, 0o600)
}

func tokenEndpoint(tenantID string) string {
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)
}

func authorizeURL(tenantID, clientID, redirectURI string) string {
	u := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize", tenantID)
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", redirectURI)
	q.Set("response_mode", "query")
	q.Set("scope", strings.Join(scopes, " "))
	q.Set("state", "openclaw-outlook")
	return u + "?" + q.Encode()
}

func postTokenForm(endpoint string, form url.Values) (tokenResponse, error) {
	var out tokenResponse
	resp, err := http.PostForm(endpoint, form)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &out); err != nil {
		return out, fmt.Errorf("token response decode: %w", err)
	}
	if resp.StatusCode >= 400 {
		d := strings.TrimSpace(out.ErrorDesc)
		if d == "" {
			d = strings.TrimSpace(string(body))
		}
		return out, fmt.Errorf("auth failed (%d): %s", resp.StatusCode, d)
	}
	if out.AccessToken == "" {
		return out, fmt.Errorf("auth failed: access_token missing")
	}
	return out, nil
}

func parseAuthCode(redirectedURL string) string {
	if strings.TrimSpace(redirectedURL) == "" {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(redirectedURL))
	if err != nil {
		return ""
	}
	return u.Query().Get("code")
}

func isTokenValid(c simpleCache) bool {
	if strings.TrimSpace(c.AccessToken) == "" {
		return false
	}
	if c.ExpiresAt == 0 {
		return true // unknown expiry from legacy cache; try it.
	}
	// 2-minute skew.
	return time.Now().Unix() < (c.ExpiresAt - 120)
}

func acquireToken(cachePath, redirectedURL, authCode string) (simpleCache, error) {
	tenantID := env("TENANT_ID")
	clientID := env("CLIENT_ID")
	clientSecret := env("CLIENT_SECRET")
	redirectURI := strings.TrimSpace(os.Getenv("REDIRECT_URI"))
	if redirectURI == "" {
		redirectURI = "http://localhost"
	}

	cache, err := loadCache(cachePath)
	if err != nil {
		return simpleCache{}, err
	}

	if isTokenValid(cache) {
		return cache, nil
	}

	endpoint := tokenEndpoint(tenantID)

	// Try refresh-token grant first when available.
	if strings.TrimSpace(cache.RefreshToken) != "" {
		form := url.Values{}
		form.Set("client_id", clientID)
		form.Set("client_secret", clientSecret)
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", cache.RefreshToken)
		form.Set("scope", strings.Join(scopes, " "))

		tr, err := postTokenForm(endpoint, form)
		if err == nil {
			newCache := simpleCache{
				AccessToken:  tr.AccessToken,
				RefreshToken: firstNonEmpty(tr.RefreshToken, cache.RefreshToken),
				ExpiresAt:    time.Now().Unix() + tr.ExpiresIn,
				TokenType:    tr.TokenType,
			}
			_ = saveCache(cachePath, newCache)
			return newCache, nil
		}
	}

	code := strings.TrimSpace(authCode)
	if code == "" {
		code = parseAuthCode(redirectedURL)
	}
	if code == "" {
		fmt.Fprintln(os.Stderr, "No valid cached token found. Starting authorization-code login flow...")
		authURL := authorizeURL(tenantID, clientID, redirectURI)
		fmt.Fprintln(os.Stderr, "Open this URL in your browser and sign in:")
		fmt.Fprintln(os.Stderr, authURL)
		fmt.Fprintln(os.Stderr, "After sign-in, copy the FULL redirected URL from browser and paste it here.")

		reader := bufio.NewReader(os.Stdin)
		fmt.Fprint(os.Stderr, "Redirected URL: ")
		line, _ := reader.ReadString('\n')
		code = parseAuthCode(strings.TrimSpace(line))
	}

	if strings.TrimSpace(code) == "" {
		return simpleCache{}, fmt.Errorf("authentication failed: missing authorization code")
	}

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("scope", strings.Join(scopes, " "))

	tr, err := postTokenForm(endpoint, form)
	if err != nil {
		return simpleCache{}, err
	}

	newCache := simpleCache{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Unix() + tr.ExpiresIn,
		TokenType:    tr.TokenType,
	}
	if err := saveCache(cachePath, newCache); err != nil {
		return simpleCache{}, err
	}
	return newCache, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func graphRequest(accessToken, method, endpoint string, payload []byte, headers map[string]string) ([]byte, int, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	blob, _ := io.ReadAll(resp.Body)
	return blob, resp.StatusCode, nil
}

func normalizePrincipal(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func bestPrincipal(me graphMe) string {
	if v := normalizePrincipal(me.UserPrincipalName); v != "" {
		return v
	}
	if v := normalizePrincipal(me.Mail); v != "" {
		return v
	}
	if v := strings.TrimSpace(me.DisplayName); v != "" {
		return v
	}
	return "unknown"
}

func getAuthenticatedUser(accessToken string) (graphMe, error) {
	var out graphMe
	q := url.Values{}
	q.Set("$select", "id,displayName,userPrincipalName,mail")
	endpoint := graphBaseURL + "/me?" + q.Encode()
	blob, code, err := graphRequest(accessToken, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return out, err
	}
	if code >= 400 {
		return out, fmt.Errorf("whoami failed: %d %s", code, strings.TrimSpace(string(blob)))
	}
	if err := json.Unmarshal(blob, &out); err != nil {
		return out, err
	}
	return out, nil
}

func ensureExpectedUser(accessToken, expected string) (graphMe, error) {
	me, err := getAuthenticatedUser(accessToken)
	if err != nil {
		return me, err
	}
	exp := normalizePrincipal(expected)
	if exp == "" {
		return me, nil
	}
	if exp == normalizePrincipal(me.UserPrincipalName) || exp == normalizePrincipal(me.Mail) {
		return me, nil
	}
	return me, fmt.Errorf("authenticated user mismatch: expected %q, got %q", exp, bestPrincipal(me))
}

func announceAuthenticatedUser(me graphMe) {
	fmt.Fprintf(os.Stderr, "AUTHENTICATED_AS=%s\n", bestPrincipal(me))
}

func buildFileAttachments(paths []string) ([]map[string]any, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	out := make([]map[string]any, 0, len(paths))
	for _, p := range paths {
		abs := expandHome(strings.TrimSpace(p))
		if abs == "" {
			continue
		}
		raw, err := os.ReadFile(abs)
		if err != nil {
			return nil, fmt.Errorf("read attachment %s: %w", abs, err)
		}
		name := filepath.Base(abs)
		ctype := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
		if ctype == "" {
			ctype = "application/octet-stream"
		}
		att := map[string]any{
			"@odata.type":  "#microsoft.graph.fileAttachment",
			"name":         name,
			"contentType":  ctype,
			"contentBytes": base64.StdEncoding.EncodeToString(raw),
		}
		out = append(out, att)
	}
	return out, nil
}

func sendMail(accessToken, to, cc, bcc, subject, body, contentType string, attachmentPaths []string) error {
	toList := parseRecipients(to)
	if len(toList) == 0 {
		return fmt.Errorf("at least one recipient is required in --to")
	}

	atts, err := buildFileAttachments(attachmentPaths)
	if err != nil {
		return err
	}

	var payload graphSendPayload
	payload.Message.Subject = subject
	payload.Message.Body.ContentType = contentType
	payload.Message.Body.Content = body
	payload.Message.ToRecipients = makeRecipients(toList)
	payload.Message.CcRecipients = makeRecipients(parseRecipients(cc))
	payload.Message.BccRecipients = makeRecipients(parseRecipients(bcc))
	payload.Message.Attachments = atts
	payload.SaveToSentItems = true

	blob, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	respBody, code, err := graphRequest(accessToken, http.MethodPost, graphBaseURL+"/me/sendMail", blob, nil)
	if err != nil {
		return err
	}
	if code != http.StatusAccepted {
		return fmt.Errorf("send failed: %d %s", code, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func listMessages(accessToken, folder, query string, top int) ([]graphMessage, error) {
	if strings.TrimSpace(folder) == "" {
		folder = "Inbox"
	}
	if top <= 0 {
		top = 20
	}
	if top > 100 {
		top = 100
	}

	q := url.Values{}
	q.Set("$top", strconv.Itoa(top))
	q.Set("$select", "id,subject,receivedDateTime,from,isRead,hasAttachments,conversationId,internetMessageId,bodyPreview")
	if strings.TrimSpace(query) == "" {
		q.Set("$orderby", "receivedDateTime DESC")
	} else {
		q.Set("$search", fmt.Sprintf("\"%s\"", strings.TrimSpace(query)))
	}

	endpoint := fmt.Sprintf("%s/me/mailFolders/%s/messages?%s", graphBaseURL, url.PathEscape(folder), q.Encode())
	headers := map[string]string{}
	if strings.TrimSpace(query) != "" {
		headers["ConsistencyLevel"] = "eventual"
	}

	blob, code, err := graphRequest(accessToken, http.MethodGet, endpoint, nil, headers)
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		return nil, fmt.Errorf("list failed: %d %s", code, strings.TrimSpace(string(blob)))
	}

	var out graphMessageList
	if err := json.Unmarshal(blob, &out); err != nil {
		return nil, err
	}
	return out.Value, nil
}

func readMessage(accessToken, messageID string) (graphMessage, error) {
	var out graphMessage
	if strings.TrimSpace(messageID) == "" {
		return out, fmt.Errorf("message id is required")
	}
	q := url.Values{}
	q.Set("$select", "id,subject,receivedDateTime,from,toRecipients,ccRecipients,isRead,hasAttachments,conversationId,internetMessageId,bodyPreview,body")
	endpoint := fmt.Sprintf("%s/me/messages/%s?%s", graphBaseURL, url.PathEscape(strings.TrimSpace(messageID)), q.Encode())
	blob, code, err := graphRequest(accessToken, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return out, err
	}
	if code >= 400 {
		return out, fmt.Errorf("read failed: %d %s", code, strings.TrimSpace(string(blob)))
	}
	if err := json.Unmarshal(blob, &out); err != nil {
		return out, err
	}
	return out, nil
}

func getAttachments(accessToken, messageID string) ([]graphAttachment, error) {
	if strings.TrimSpace(messageID) == "" {
		return nil, fmt.Errorf("message id is required")
	}
	endpoint := fmt.Sprintf("%s/me/messages/%s/attachments?$top=200", graphBaseURL, url.PathEscape(strings.TrimSpace(messageID)))
	blob, code, err := graphRequest(accessToken, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		return nil, fmt.Errorf("attachments read failed: %d %s", code, strings.TrimSpace(string(blob)))
	}
	var out graphAttachmentList
	if err := json.Unmarshal(blob, &out); err != nil {
		return nil, err
	}
	return out.Value, nil
}

func downloadAttachments(accessToken, messageID, outDir string) ([]string, error) {
	atts, err := getAttachments(accessToken, messageID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(outDir) == "" {
		outDir = "."
	}
	outDir = expandHome(outDir)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	saved := make([]string, 0, len(atts))
	for _, a := range atts {
		if !strings.Contains(a.ODataType, "fileAttachment") {
			continue
		}
		if strings.TrimSpace(a.ContentBytes) == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(a.ContentBytes)
		if err != nil {
			return nil, fmt.Errorf("decode attachment %s: %w", a.Name, err)
		}
		name := strings.TrimSpace(a.Name)
		if name == "" {
			name = "attachment-" + a.ID
		}
		target := filepath.Join(outDir, filepath.Base(name))
		if err := os.WriteFile(target, raw, 0o644); err != nil {
			return nil, err
		}
		saved = append(saved, target)
	}
	return saved, nil
}

func replyMessage(accessToken, messageID, body, contentType string, replyAll bool) error {
	if strings.TrimSpace(messageID) == "" {
		return fmt.Errorf("message id is required")
	}
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("reply body is required")
	}

	endpoint := fmt.Sprintf("%s/me/messages/%s/reply", graphBaseURL, url.PathEscape(strings.TrimSpace(messageID)))
	if replyAll {
		endpoint = fmt.Sprintf("%s/me/messages/%s/replyAll", graphBaseURL, url.PathEscape(strings.TrimSpace(messageID)))
	}

	payload := map[string]any{
		"message": map[string]any{
			"body": map[string]any{
				"contentType": contentType,
				"content":     body,
			},
		},
	}
	blob, _ := json.Marshal(payload)
	respBody, code, err := graphRequest(accessToken, http.MethodPost, endpoint, blob, nil)
	if err != nil {
		return err
	}
	if code != http.StatusAccepted {
		return fmt.Errorf("reply failed: %d %s", code, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func recipientLabel(r graphRecipient) string {
	addr := strings.TrimSpace(r.EmailAddress.Address)
	name := strings.TrimSpace(r.EmailAddress.Name)
	if name != "" && addr != "" {
		return fmt.Sprintf("%s <%s>", name, addr)
	}
	if addr != "" {
		return addr
	}
	return name
}

func parsePathList(value string) []string {
	value = strings.ReplaceAll(value, ";", ",")
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func usage() {
	fmt.Println("outlook-mail (Go)")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  send                 Send email (default when flags are provided directly)")
	fmt.Println("  list                 List/search inbox messages")
	fmt.Println("  read                 Read one message")
	fmt.Println("  download-attachments Download attachments from a message")
	fmt.Println("  reply                Reply to a message (or reply-all)")
	fmt.Println("  whoami               Print authenticated mailbox identity")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  go run . --to \"a@b.com\" --subject \"Hi\" --body \"<p>Hello</p>\"")
	fmt.Println("  go run . send --to \"a@b.com\" --subject \"Hi\" --body \"<p>Hello</p>\" --attach \"/tmp/a.pdf,/tmp/b.csv\"")
	fmt.Println("  go run . list --folder Inbox --top 20")
	fmt.Println("  go run . whoami --profile risk-agent")
	fmt.Println("  go run . list --query \"from:y.borisova@hksglobal.group\"")
	fmt.Println("  go run . list --profile risk-agent --expect-user risk-agent@hksglobal.group --top 10")
	fmt.Println("  go run . read --id <message-id> --expect-user henry@hksglobal.group")
	fmt.Println("  go run . download-attachments --id <message-id> --out ./downloads")
	fmt.Println("  go run . reply --id <message-id> --body \"<p>Thanks</p>\" --reply-all")
}

func cmdSend(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)

	to := fs.String("to", "", "Recipient email(s), comma/semicolon separated")
	cc := fs.String("cc", "", "CC email(s), comma/semicolon separated")
	bcc := fs.String("bcc", "", "BCC email(s), comma/semicolon separated")
	subject := fs.String("subject", "", "Email subject")
	body := fs.String("body", "", "Email body text/HTML")
	plainText := fs.Bool("text", false, "Send as plain text instead of HTML")
	attach := fs.String("attach", "", "Attachment path(s), comma/semicolon separated")
	profile := fs.String("profile", "", "Token cache profile (e.g. risk-agent). Uses OUTLOOK_PROFILE when omitted")
	cachePath := fs.String("cache", "", "Path to token cache file (overrides --profile)")
	expectUser := fs.String("expect-user", "", "Expected mailbox identity (UPN/email). Fails on mismatch")
	redirectedURL := fs.String("redirected-url", "", "Optional full redirect URL from browser after auth")
	authCode := fs.String("auth-code", "", "Optional OAuth authorization code")
	_ = fs.Parse(args)

	if strings.TrimSpace(*to) == "" {
		fatalf("missing required flag: --to")
	}
	if strings.TrimSpace(*subject) == "" {
		fatalf("missing required flag: --subject")
	}
	if strings.TrimSpace(*body) == "" {
		fatalf("missing required flag: --body")
	}

	resolvedCache := expandHome(strings.TrimSpace(*cachePath))
	if resolvedCache == "" {
		resolvedCache = defaultOutlookCachePath(*profile)
	}

	token, err := acquireToken(resolvedCache, *redirectedURL, *authCode)
	if err != nil {
		fatalf("Authentication failed: %v", err)
	}
	me, err := ensureExpectedUser(token.AccessToken, *expectUser)
	if err != nil {
		fatalf("Identity check failed: %v", err)
	}
	announceAuthenticatedUser(me)

	contentType := "HTML"
	if *plainText {
		contentType = "Text"
	}

	attachmentPaths := parsePathList(*attach)
	if err := sendMail(token.AccessToken, *to, *cc, *bcc, *subject, *body, contentType, attachmentPaths); err != nil {
		fatalf("%v", err)
	}

	details := make([]string, 0, 3)
	if strings.TrimSpace(*cc) != "" {
		details = append(details, "cc: "+*cc)
	}
	if strings.TrimSpace(*bcc) != "" {
		details = append(details, "bcc: "+*bcc)
	}
	if len(attachmentPaths) > 0 {
		details = append(details, fmt.Sprintf("attachments: %d", len(attachmentPaths)))
	}

	if len(details) > 0 {
		fmt.Printf("Email queued successfully to %s (%s)\n", *to, strings.Join(details, "; "))
	} else {
		fmt.Printf("Email queued successfully to %s\n", *to)
	}
}

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)

	folder := fs.String("folder", "Inbox", "Mail folder name (Inbox, Sent Items, etc.)")
	top := fs.Int("top", 20, "Number of messages to fetch (1-100)")
	query := fs.String("query", "", "Optional Graph search query (e.g. from:user@x.com subject:risk)")
	asJSON := fs.Bool("json", false, "Print JSON output")
	profile := fs.String("profile", "", "Token cache profile (e.g. risk-agent). Uses OUTLOOK_PROFILE when omitted")
	cachePath := fs.String("cache", "", "Path to token cache file (overrides --profile)")
	expectUser := fs.String("expect-user", "", "Expected mailbox identity (UPN/email). Fails on mismatch")
	redirectedURL := fs.String("redirected-url", "", "Optional full redirect URL from browser after auth")
	authCode := fs.String("auth-code", "", "Optional OAuth authorization code")
	_ = fs.Parse(args)

	resolvedCache := expandHome(strings.TrimSpace(*cachePath))
	if resolvedCache == "" {
		resolvedCache = defaultOutlookCachePath(*profile)
	}

	token, err := acquireToken(resolvedCache, *redirectedURL, *authCode)
	if err != nil {
		fatalf("Authentication failed: %v", err)
	}
	me, err := ensureExpectedUser(token.AccessToken, *expectUser)
	if err != nil {
		fatalf("Identity check failed: %v", err)
	}
	announceAuthenticatedUser(me)

	msgs, err := listMessages(token.AccessToken, *folder, *query, *top)
	if err != nil {
		fatalf("%v", err)
	}

	if *asJSON {
		blob, _ := json.MarshalIndent(msgs, "", "  ")
		fmt.Println(string(blob))
		return
	}

	for _, m := range msgs {
		flags := []string{}
		if !m.IsRead {
			flags = append(flags, "UNREAD")
		}
		if m.HasAttachments {
			flags = append(flags, "ATTACH")
		}
		tag := ""
		if len(flags) > 0 {
			tag = " [" + strings.Join(flags, ",") + "]"
		}
		fmt.Printf("%s | %s | %s | %s%s\n", m.ID, m.ReceivedDateTime, recipientLabel(m.From), m.Subject, tag)
	}
}

func cmdRead(args []string) {
	fs := flag.NewFlagSet("read", flag.ExitOnError)

	id := fs.String("id", "", "Message ID")
	asJSON := fs.Bool("json", false, "Print JSON output")
	showAttachments := fs.Bool("attachments", false, "Also list attachment metadata")
	profile := fs.String("profile", "", "Token cache profile (e.g. risk-agent). Uses OUTLOOK_PROFILE when omitted")
	cachePath := fs.String("cache", "", "Path to token cache file (overrides --profile)")
	expectUser := fs.String("expect-user", "", "Expected mailbox identity (UPN/email). Fails on mismatch")
	redirectedURL := fs.String("redirected-url", "", "Optional full redirect URL from browser after auth")
	authCode := fs.String("auth-code", "", "Optional OAuth authorization code")
	_ = fs.Parse(args)

	if strings.TrimSpace(*id) == "" {
		fatalf("missing required flag: --id")
	}

	resolvedCache := expandHome(strings.TrimSpace(*cachePath))
	if resolvedCache == "" {
		resolvedCache = defaultOutlookCachePath(*profile)
	}

	token, err := acquireToken(resolvedCache, *redirectedURL, *authCode)
	if err != nil {
		fatalf("Authentication failed: %v", err)
	}
	me, err := ensureExpectedUser(token.AccessToken, *expectUser)
	if err != nil {
		fatalf("Identity check failed: %v", err)
	}
	announceAuthenticatedUser(me)

	msg, err := readMessage(token.AccessToken, *id)
	if err != nil {
		fatalf("%v", err)
	}

	if *asJSON {
		payload := map[string]any{"message": msg}
		if *showAttachments {
			atts, _ := getAttachments(token.AccessToken, *id)
			payload["attachments"] = atts
		}
		blob, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(blob))
		return
	}

	fmt.Printf("ID: %s\n", msg.ID)
	fmt.Printf("Received: %s\n", msg.ReceivedDateTime)
	fmt.Printf("From: %s\n", recipientLabel(msg.From))
	fmt.Printf("To: %s\n", joinRecipients(msg.ToRecipients))
	if len(msg.CcRecipients) > 0 {
		fmt.Printf("CC: %s\n", joinRecipients(msg.CcRecipients))
	}
	fmt.Printf("Subject: %s\n", msg.Subject)
	fmt.Printf("ConversationID: %s\n", msg.ConversationID)
	fmt.Printf("InternetMessageID: %s\n", msg.InternetMessageID)
	fmt.Printf("HasAttachments: %v\n", msg.HasAttachments)
	fmt.Printf("BodyType: %s\n\n", msg.Body.ContentType)
	fmt.Println(msg.Body.Content)

	if *showAttachments {
		atts, err := getAttachments(token.AccessToken, *id)
		if err != nil {
			fatalf("attachments: %v", err)
		}
		fmt.Println("\nAttachments:")
		if len(atts) == 0 {
			fmt.Println("  (none)")
		}
		for _, a := range atts {
			fmt.Printf("- %s | %s | %d bytes | inline=%v\n", a.Name, a.ContentType, a.Size, a.IsInline)
		}
	}
}

func cmdDownloadAttachments(args []string) {
	fs := flag.NewFlagSet("download-attachments", flag.ExitOnError)

	id := fs.String("id", "", "Message ID")
	outDir := fs.String("out", "./downloads", "Output directory")
	profile := fs.String("profile", "", "Token cache profile (e.g. risk-agent). Uses OUTLOOK_PROFILE when omitted")
	cachePath := fs.String("cache", "", "Path to token cache file (overrides --profile)")
	expectUser := fs.String("expect-user", "", "Expected mailbox identity (UPN/email). Fails on mismatch")
	redirectedURL := fs.String("redirected-url", "", "Optional full redirect URL from browser after auth")
	authCode := fs.String("auth-code", "", "Optional OAuth authorization code")
	_ = fs.Parse(args)

	if strings.TrimSpace(*id) == "" {
		fatalf("missing required flag: --id")
	}

	resolvedCache := expandHome(strings.TrimSpace(*cachePath))
	if resolvedCache == "" {
		resolvedCache = defaultOutlookCachePath(*profile)
	}

	token, err := acquireToken(resolvedCache, *redirectedURL, *authCode)
	if err != nil {
		fatalf("Authentication failed: %v", err)
	}
	me, err := ensureExpectedUser(token.AccessToken, *expectUser)
	if err != nil {
		fatalf("Identity check failed: %v", err)
	}
	announceAuthenticatedUser(me)

	saved, err := downloadAttachments(token.AccessToken, *id, *outDir)
	if err != nil {
		fatalf("%v", err)
	}
	if len(saved) == 0 {
		fmt.Println("No downloadable file attachments found.")
		return
	}
	for _, p := range saved {
		fmt.Printf("SAVED: %s\n", p)
	}
}

func cmdReply(args []string) {
	fs := flag.NewFlagSet("reply", flag.ExitOnError)

	id := fs.String("id", "", "Message ID")
	body := fs.String("body", "", "Reply body")
	plainText := fs.Bool("text", false, "Send as plain text instead of HTML")
	replyAll := fs.Bool("reply-all", false, "Use replyAll instead of reply")
	profile := fs.String("profile", "", "Token cache profile (e.g. risk-agent). Uses OUTLOOK_PROFILE when omitted")
	cachePath := fs.String("cache", "", "Path to token cache file (overrides --profile)")
	expectUser := fs.String("expect-user", "", "Expected mailbox identity (UPN/email). Fails on mismatch")
	redirectedURL := fs.String("redirected-url", "", "Optional full redirect URL from browser after auth")
	authCode := fs.String("auth-code", "", "Optional OAuth authorization code")
	_ = fs.Parse(args)

	if strings.TrimSpace(*id) == "" {
		fatalf("missing required flag: --id")
	}
	if strings.TrimSpace(*body) == "" {
		fatalf("missing required flag: --body")
	}

	resolvedCache := expandHome(strings.TrimSpace(*cachePath))
	if resolvedCache == "" {
		resolvedCache = defaultOutlookCachePath(*profile)
	}

	token, err := acquireToken(resolvedCache, *redirectedURL, *authCode)
	if err != nil {
		fatalf("Authentication failed: %v", err)
	}
	me, err := ensureExpectedUser(token.AccessToken, *expectUser)
	if err != nil {
		fatalf("Identity check failed: %v", err)
	}
	announceAuthenticatedUser(me)

	contentType := "HTML"
	if *plainText {
		contentType = "Text"
	}

	if err := replyMessage(token.AccessToken, *id, *body, contentType, *replyAll); err != nil {
		fatalf("%v", err)
	}
	if *replyAll {
		fmt.Printf("Reply-all queued successfully for message %s\n", *id)
	} else {
		fmt.Printf("Reply queued successfully for message %s\n", *id)
	}
}

func cmdWhoAmI(args []string) {
	fs := flag.NewFlagSet("whoami", flag.ExitOnError)

	asJSON := fs.Bool("json", false, "Print JSON output")
	profile := fs.String("profile", "", "Token cache profile (e.g. risk-agent). Uses OUTLOOK_PROFILE when omitted")
	cachePath := fs.String("cache", "", "Path to token cache file (overrides --profile)")
	expectUser := fs.String("expect-user", "", "Expected mailbox identity (UPN/email). Fails on mismatch")
	redirectedURL := fs.String("redirected-url", "", "Optional full redirect URL from browser after auth")
	authCode := fs.String("auth-code", "", "Optional OAuth authorization code")
	_ = fs.Parse(args)

	resolvedCache := expandHome(strings.TrimSpace(*cachePath))
	if resolvedCache == "" {
		resolvedCache = defaultOutlookCachePath(*profile)
	}

	token, err := acquireToken(resolvedCache, *redirectedURL, *authCode)
	if err != nil {
		fatalf("Authentication failed: %v", err)
	}
	me, err := ensureExpectedUser(token.AccessToken, *expectUser)
	if err != nil {
		fatalf("Identity check failed: %v", err)
	}
	announceAuthenticatedUser(me)

	if *asJSON {
		blob, _ := json.MarshalIndent(me, "", "  ")
		fmt.Println(string(blob))
		return
	}

	fmt.Printf("UserPrincipalName: %s\n", strings.TrimSpace(me.UserPrincipalName))
	fmt.Printf("Mail: %s\n", strings.TrimSpace(me.Mail))
	fmt.Printf("DisplayName: %s\n", strings.TrimSpace(me.DisplayName))
	fmt.Printf("ID: %s\n", strings.TrimSpace(me.ID))
}

func joinRecipients(rs []graphRecipient) string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		v := recipientLabel(r)
		if v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, ", ")
}

func main() {
	if len(os.Args) == 1 {
		usage()
		os.Exit(2)
	}

	cmd := "send"
	args := os.Args[1:]
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		cmd = strings.ToLower(strings.TrimSpace(os.Args[1]))
		args = os.Args[2:]
	}

	switch cmd {
	case "send":
		cmdSend(args)
	case "list":
		cmdList(args)
	case "read":
		cmdRead(args)
	case "download-attachments", "attachments":
		cmdDownloadAttachments(args)
	case "reply":
		cmdReply(args)
	case "whoami":
		cmdWhoAmI(args)
	case "help", "-h", "--help":
		usage()
	default:
		fatalf("unknown command: %s\n\nRun with 'help' for usage.", cmd)
	}
}

func mustHome() string {
	h, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(h) == "" {
		return "."
	}
	return h
}

func sanitizeProfile(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		if r == '-' || r == '_' || r == '.' {
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func defaultOutlookCachePath(profile string) string {
	profile = firstNonEmpty(profile, os.Getenv("OUTLOOK_PROFILE"))
	profile = sanitizeProfile(profile)
	if profile == "" || profile == "default" {
		return filepath.Join(mustHome(), ".openclaw", "outlook_token_cache.json")
	}
	return filepath.Join(mustHome(), ".openclaw", fmt.Sprintf("outlook_token_cache_%s.json", profile))
}
