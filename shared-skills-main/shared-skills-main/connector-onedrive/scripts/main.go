package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
)

const graphBase = "https://graph.microsoft.com/v1.0"

var defaultScopes = []string{"User.Read", "Files.ReadWrite"}

// --------------------------
// Helpers: env file loading
// --------------------------

func loadEnvFile(p string) {
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, "\"'")
		if k != "" {
			if _, exists := os.LookupEnv(k); !exists {
				_ = os.Setenv(k, v)
			}
		}
	}
}

// --------------------------
// MSAL cache to file
// --------------------------

type fileCache struct {
	path string
}

func (f fileCache) Replace(ctx context.Context, c cache.Unmarshaler, _ cache.ReplaceHints) error {
	// best-effort timeout if caller didn't set one
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	b, err := os.ReadFile(f.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	return c.Unmarshal(b)
}

func (f fileCache) Export(ctx context.Context, c cache.Marshaler, _ cache.ExportHints) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	b, err := c.Marshal()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(f.path, b, 0o600)
}

// --------------------------
// Auth management
// --------------------------

type authConfig struct {
	tenantID     string
	clientID     string
	clientSecret string
	redirectURI  string
	scopes       []string
	cachePath    string
	accountPath  string
}

func (c authConfig) authority() string {
	return fmt.Sprintf("https://login.microsoftonline.com/%s", c.tenantID)
}

func acquireAccessToken(ctx context.Context, cfg authConfig) (string, error) {
	if cfg.tenantID == "" || cfg.clientID == "" {
		return "", fmt.Errorf("missing TENANT_ID / CLIENT_ID")
	}

	// If client secret is present, prefer confidential auth-code flow (matches existing Python behavior),
	// but first attempt silent reuse of any existing public-client token cache.
	if cfg.clientSecret != "" {
		if token, err := acquireAccessTokenFromPublicCache(ctx, cfg); err == nil && token != "" {
			return token, nil
		}
		cred, err := confidential.NewCredFromSecret(cfg.clientSecret)
		if err != nil {
			return "", err
		}
		cca, err := confidential.New(cfg.authority(), cfg.clientID, cred, confidential.WithCache(fileCache{path: cfg.cachePath}))
		if err != nil {
			return "", err
		}

		// Try silent
		if homeID := readHomeAccountID(cfg.accountPath); homeID != "" {
			acc, err := cca.Account(ctx, homeID)
			if err == nil {
				res, err := cca.AcquireTokenSilent(ctx, cfg.scopes, confidential.WithSilentAccount(acc))
				if err == nil && res.AccessToken != "" {
					return res.AccessToken, nil
				}
			}
		}

		// Interactive via auth-code
		authURL, err := cca.AuthCodeURL(ctx, cfg.clientID, cfg.redirectURI, cfg.scopes)
		if err != nil {
			return "", err
		}

		fmt.Fprintln(os.Stderr, "\n=== BROWSER LOGIN REQUIRED (OneDrive scopes) ===")
		fmt.Fprintln(os.Stderr, "Open this URL and sign in/consent:")
		fmt.Fprintln(os.Stderr, authURL)
		fmt.Fprintln(os.Stderr, "\nPaste the FULL redirected URL (http://localhost/?code=...):")

		redir, err := readLine("Redirected URL: ")
		if err != nil {
			return "", err
		}
		code := extractCodeFromRedirect(redir)
		if code == "" {
			return "", fmt.Errorf("could not extract code from redirected URL")
		}

		res, err := cca.AcquireTokenByAuthCode(ctx, code, cfg.redirectURI, cfg.scopes)
		if err != nil {
			return "", err
		}
		if res.AccessToken == "" {
			return "", fmt.Errorf("no access token returned")
		}

		writeHomeAccountID(cfg.accountPath, res.Account.HomeAccountID)
		return res.AccessToken, nil
	}

	// Public client: device code flow
	pca, err := public.New(cfg.clientID, public.WithAuthority(cfg.authority()), public.WithCache(fileCache{path: cfg.cachePath}))
	if err != nil {
		return "", err
	}

	// Try silent using first cached account
	accounts, _ := pca.Accounts(ctx)
	if len(accounts) > 0 {
		res, err := pca.AcquireTokenSilent(ctx, cfg.scopes, public.WithSilentAccount(accounts[0]))
		if err == nil && res.AccessToken != "" {
			return res.AccessToken, nil
		}
	}

	dc, err := pca.AcquireTokenByDeviceCode(ctx, cfg.scopes)
	if err != nil {
		return "", err
	}
	// dc.Result usually has a Message; print what we can.
	fmt.Fprintln(os.Stderr, "\n=== DEVICE LOGIN REQUIRED (OneDrive scopes) ===")
	fmt.Fprintln(os.Stderr, "Follow the instructions below to authenticate:")
	b, _ := json.MarshalIndent(dc.Result, "", "  ")
	fmt.Fprintln(os.Stderr, string(b))
	res, err := dc.AuthenticationResult(ctx)
	if err != nil {
		return "", err
	}
	if res.AccessToken == "" {
		return "", fmt.Errorf("no access token returned")
	}
	return res.AccessToken, nil
}

func acquireAccessTokenFromPublicCache(ctx context.Context, cfg authConfig) (string, error) {
	pca, err := public.New(cfg.clientID, public.WithAuthority(cfg.authority()), public.WithCache(fileCache{path: cfg.cachePath}))
	if err != nil {
		return "", err
	}
	accounts, err := pca.Accounts(ctx)
	if err != nil || len(accounts) == 0 {
		return "", fmt.Errorf("no cached public accounts")
	}
	for _, acc := range accounts {
		res, err := pca.AcquireTokenSilent(ctx, cfg.scopes, public.WithSilentAccount(acc))
		if err == nil && res.AccessToken != "" {
			return res.AccessToken, nil
		}
	}
	return "", fmt.Errorf("no reusable cached public token")
}

func extractCodeFromRedirect(redirectedURL string) string {
	redirectedURL = strings.TrimSpace(redirectedURL)
	u, err := url.Parse(redirectedURL)
	if err != nil {
		return ""
	}
	q := u.Query()
	return q.Get("code")
}

func readLine(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	in := bufio.NewReader(os.Stdin)
	s, err := in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(s), nil
}

type accountFile struct {
	HomeAccountID string `json:"homeAccountId"`
}

func readHomeAccountID(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	var af accountFile
	if err := json.Unmarshal(b, &af); err != nil {
		return ""
	}
	return strings.TrimSpace(af.HomeAccountID)
}

func writeHomeAccountID(p, id string) {
	if id == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	b, _ := json.MarshalIndent(accountFile{HomeAccountID: id}, "", "  ")
	_ = os.WriteFile(p, b, 0o600)
}

// --------------------------
// Graph client
// --------------------------

type graphClient struct {
	http        *http.Client
	accessToken string
	timeout     time.Duration
}

func newGraphClient(accessToken string) *graphClient {
	return &graphClient{
		http:        &http.Client{},
		accessToken: accessToken,
		timeout:     60 * time.Second,
	}
}

func (g *graphClient) doJSON(ctx context.Context, method, urlStr string, body any, out any, headers map[string]string) (*http.Response, []byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		rdr = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, rdr)
	if err != nil {
		return nil, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.accessToken)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := g.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, err
	}
	if resp.StatusCode >= 400 {
		return resp, b, fmt.Errorf("graph error %d: %s", resp.StatusCode, truncate(string(b), 800))
	}
	if out != nil && len(b) > 0 {
		if err := json.Unmarshal(b, out); err != nil {
			return resp, b, err
		}
	}
	return resp, b, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

type driveItem struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Size            int64  `json:"size"`
	WebURL          string `json:"webUrl"`
	Folder          any    `json:"folder"`
	File            any    `json:"file"`
	ParentReference struct {
		ID      string `json:"id"`
		Path    string `json:"path"`
		DriveID string `json:"driveId"`
	} `json:"parentReference"`
}

func parseDriveItemRef(target string) (driveID, itemID string, ok bool) {
	target = strings.TrimSpace(target)
	if !strings.HasPrefix(target, "drive:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(target, "drive:")
	parts := strings.SplitN(rest, ":item:", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	driveID = strings.TrimSpace(parts[0])
	itemID = strings.TrimSpace(parts[1])
	if driveID == "" || itemID == "" {
		return "", "", false
	}
	return driveID, itemID, true
}

type resolvedTarget struct {
	Item         driveItem
	DriveID      string
	ChildrenURL  string
	ContentURL   string
	CanonicalURL string
}

func isHTTPURL(target string) bool {
	t := strings.ToLower(strings.TrimSpace(target))
	return strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://")
}

func encodeSharingURL(raw string) string {
	b := base64.StdEncoding.EncodeToString([]byte(raw))
	b = strings.TrimRight(b, "=")
	b = strings.ReplaceAll(b, "/", "_")
	b = strings.ReplaceAll(b, "+", "-")
	return "u!" + b
}

func splitSharePointServerRelativePath(serverRel string) (sitePath, itemPath string, ok bool) {
	serverRel, err := url.PathUnescape(strings.TrimSpace(serverRel))
	if err != nil {
		serverRel = strings.TrimSpace(serverRel)
	}
	serverRel = path.Clean("/" + strings.TrimPrefix(serverRel, "/"))
	parts := strings.Split(strings.TrimPrefix(serverRel, "/"), "/")
	if len(parts) < 3 {
		return "", "", false
	}
	if parts[0] != "sites" && parts[0] != "teams" && parts[0] != "personal" {
		return "", "", false
	}
	if parts[0] == "personal" {
		if len(parts) < 3 {
			return "", "", false
		}
		sitePath = "/" + path.Join(parts[0], parts[1])
		parts = parts[2:]
	} else {
		sitePath = "/" + path.Join(parts[0], parts[1])
		parts = parts[2:]
	}
	if len(parts) == 0 {
		return sitePath, "/", true
	}
	if strings.EqualFold(parts[0], "Shared Documents") || strings.EqualFold(parts[0], "Documents") {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return sitePath, "/", true
	}
	return sitePath, "/" + path.Join(parts...), true
}

type listChildrenResp struct {
	Value    []driveItem `json:"value"`
	NextLink string      `json:"@odata.nextLink"`
}

func encodePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = path.Clean(p)
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	enc := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		enc = append(enc, url.PathEscape(part))
	}
	return "/" + strings.Join(enc, "/")
}

func (g *graphClient) itemByPath(ctx context.Context, p string) (driveItem, error) {
	if strings.TrimSpace(p) == "" || strings.TrimSpace(p) == "/" {
		var out driveItem
		_, _, err := g.doJSON(ctx, "GET", graphBase+"/me/drive/root", nil, &out, nil)
		return out, err
	}
	enc := encodePath(p)
	var out driveItem
	_, _, err := g.doJSON(ctx, "GET", graphBase+"/me/drive/root:"+enc, nil, &out, nil)
	return out, err
}

func (g *graphClient) itemByID(ctx context.Context, id string) (driveItem, error) {
	var out driveItem
	_, _, err := g.doJSON(ctx, "GET", graphBase+"/me/drive/items/"+url.PathEscape(id), nil, &out, nil)
	return out, err
}

func (g *graphClient) itemByDriveAndID(ctx context.Context, driveID, id string) (driveItem, error) {
	var out driveItem
	_, _, err := g.doJSON(ctx, "GET", graphBase+"/drives/"+url.PathEscape(driveID)+"/items/"+url.PathEscape(id), nil, &out, nil)
	return out, err
}

func (g *graphClient) resolveShareURL(ctx context.Context, rawURL string) (resolvedTarget, error) {
	shareToken := encodeSharingURL(strings.TrimSpace(rawURL))
	base := graphBase + "/shares/" + shareToken + "/driveItem"
	var out driveItem
	_, _, err := g.doJSON(ctx, "GET", base, nil, &out, nil)
	if err == nil {
		return resolvedTarget{
			Item:         out,
			DriveID:      out.ParentReference.DriveID,
			ChildrenURL:  base + "/children?$top=200&$select=id,name,size,folder,file,lastModifiedDateTime,parentReference,webUrl",
			ContentURL:   base + "/content",
			CanonicalURL: rawURL,
		}, nil
	}

	u, parseErr := url.Parse(strings.TrimSpace(rawURL))
	if parseErr != nil {
		return resolvedTarget{}, err
	}
	if !strings.HasSuffix(strings.ToLower(u.Hostname()), ".sharepoint.com") {
		return resolvedTarget{}, err
	}
	serverRel := strings.TrimSpace(u.Query().Get("id"))
	if serverRel == "" {
		return resolvedTarget{}, err
	}
	sitePath, itemPath, ok := splitSharePointServerRelativePath(serverRel)
	if !ok {
		return resolvedTarget{}, err
	}
	var site struct {
		ID string `json:"id"`
	}
	_, _, siteErr := g.doJSON(ctx, "GET", graphBase+"/sites/"+u.Hostname()+":"+sitePath, nil, &site, nil)
	if siteErr != nil {
		return resolvedTarget{}, fmt.Errorf("share-url resolve failed (%v); sharepoint path fallback failed (%v)", err, siteErr)
	}
	var item driveItem
	itemURL := graphBase + "/sites/" + url.PathEscape(site.ID) + "/drive/root"
	if itemPath != "/" {
		itemURL += ":" + encodePath(itemPath)
	}
	_, _, itemErr := g.doJSON(ctx, "GET", itemURL, nil, &item, nil)
	if itemErr != nil {
		return resolvedTarget{}, fmt.Errorf("share-url resolve failed (%v); sharepoint path item fallback failed (%v)", err, itemErr)
	}
	childrenURL := graphBase + "/drives/" + url.PathEscape(item.ParentReference.DriveID) + "/items/" + url.PathEscape(item.ID) + "/children?$top=200&$select=id,name,size,folder,file,lastModifiedDateTime,parentReference,webUrl"
	contentURL := graphBase + "/drives/" + url.PathEscape(item.ParentReference.DriveID) + "/items/" + url.PathEscape(item.ID) + "/content"
	return resolvedTarget{Item: item, DriveID: item.ParentReference.DriveID, ChildrenURL: childrenURL, ContentURL: contentURL, CanonicalURL: rawURL}, nil
}

func (g *graphClient) resolveTarget(ctx context.Context, target string) (resolvedTarget, error) {
	target = strings.TrimSpace(target)
	if isHTTPURL(target) {
		return g.resolveShareURL(ctx, target)
	}
	if driveID, itemID, ok := parseDriveItemRef(target); ok {
		it, err := g.itemByDriveAndID(ctx, driveID, itemID)
		if err != nil {
			return resolvedTarget{}, err
		}
		childrenURL := graphBase + "/drives/" + url.PathEscape(driveID) + "/items/" + url.PathEscape(it.ID) + "/children?$top=200&$select=id,name,size,folder,file,lastModifiedDateTime,parentReference,webUrl"
		contentURL := graphBase + "/drives/" + url.PathEscape(driveID) + "/items/" + url.PathEscape(it.ID) + "/content"
		return resolvedTarget{Item: it, DriveID: driveID, ChildrenURL: childrenURL, ContentURL: contentURL, CanonicalURL: target}, nil
	}
	if strings.HasPrefix(target, "id:") {
		id := strings.TrimPrefix(target, "id:")
		it, err := g.itemByID(ctx, id)
		if err != nil {
			return resolvedTarget{}, err
		}
		driveID := it.ParentReference.DriveID
		childrenURL := graphBase + "/me/drive/items/" + url.PathEscape(it.ID) + "/children?$top=200&$select=id,name,size,folder,file,lastModifiedDateTime,parentReference,webUrl"
		contentURL := graphBase + "/me/drive/items/" + url.PathEscape(it.ID) + "/content"
		if driveID != "" {
			childrenURL = graphBase + "/drives/" + url.PathEscape(driveID) + "/items/" + url.PathEscape(it.ID) + "/children?$top=200&$select=id,name,size,folder,file,lastModifiedDateTime,parentReference,webUrl"
			contentURL = graphBase + "/drives/" + url.PathEscape(driveID) + "/items/" + url.PathEscape(it.ID) + "/content"
		}
		return resolvedTarget{Item: it, DriveID: driveID, ChildrenURL: childrenURL, ContentURL: contentURL, CanonicalURL: target}, nil
	}
	it, err := g.itemByPath(ctx, target)
	if err != nil {
		return resolvedTarget{}, err
	}
	driveID := it.ParentReference.DriveID
	childrenURL := graphBase + "/me/drive/items/" + url.PathEscape(it.ID) + "/children?$top=200&$select=id,name,size,folder,file,lastModifiedDateTime,parentReference,webUrl"
	contentURL := graphBase + "/me/drive/items/" + url.PathEscape(it.ID) + "/content"
	if driveID != "" {
		childrenURL = graphBase + "/drives/" + url.PathEscape(driveID) + "/items/" + url.PathEscape(it.ID) + "/children?$top=200&$select=id,name,size,folder,file,lastModifiedDateTime,parentReference,webUrl"
		contentURL = graphBase + "/drives/" + url.PathEscape(driveID) + "/items/" + url.PathEscape(it.ID) + "/content"
	}
	return resolvedTarget{Item: it, DriveID: driveID, ChildrenURL: childrenURL, ContentURL: contentURL, CanonicalURL: target}, nil
}

func (g *graphClient) resolveItem(ctx context.Context, target string) (driveItem, error) {
	rt, err := g.resolveTarget(ctx, target)
	if err != nil {
		return driveItem{}, err
	}
	return rt.Item, nil
}

func (g *graphClient) listChildren(ctx context.Context, target string) ([]driveItem, error) {
	rt, err := g.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	if rt.Item.Folder == nil {
		return nil, fmt.Errorf("not a folder: %s", target)
	}
	return g.listChildrenURL(ctx, rt.ChildrenURL)
}

func (g *graphClient) listChildrenURL(ctx context.Context, urlStr string) ([]driveItem, error) {
	out := make([]driveItem, 0)
	for urlStr != "" {
		var page listChildrenResp
		_, _, err := g.doJSON(ctx, "GET", urlStr, nil, &page, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Value...)
		urlStr = page.NextLink
	}
	return out, nil
}

func (g *graphClient) listChildrenByID(ctx context.Context, folderID string) ([]driveItem, error) {
	urlStr := graphBase + "/me/drive/items/" + url.PathEscape(folderID) + "/children?$top=200&$select=id,name,size,folder,file,lastModifiedDateTime,parentReference,webUrl"
	return g.listChildrenURL(ctx, urlStr)
}

func (g *graphClient) ensureFolderPath(ctx context.Context, folderPath string) (driveItem, error) {
	folderPath = strings.TrimSpace(folderPath)
	if folderPath == "" || folderPath == "/" {
		return g.itemByPath(ctx, "/")
	}
	if !strings.HasPrefix(folderPath, "/") {
		folderPath = "/" + folderPath
	}
	folderPath = path.Clean(folderPath)

	segments := strings.Split(strings.TrimPrefix(folderPath, "/"), "/")
	curPath := "/"
	curItem, err := g.itemByPath(ctx, "/")
	if err != nil {
		return driveItem{}, err
	}

	for _, seg := range segments {
		if seg == "" {
			continue
		}
		curPath = path.Join(curPath, seg)
		nextItem, err := g.itemByPath(ctx, curPath)
		if err == nil {
			if nextItem.Folder == nil {
				return driveItem{}, fmt.Errorf("path exists but is not folder: %s", curPath)
			}
			curItem = nextItem
			continue
		}

		created, err := createFolder(ctx, g, curItem.ID, seg)
		if err != nil {
			return driveItem{}, err
		}
		curItem = created
	}
	return curItem, nil
}

// --------------------------
// Command implementations
// --------------------------

type config struct {
	envFile      string
	tenantID     string
	clientID     string
	clientSecret string
	redirectURI  string
	cachePath    string
	jsonOut      bool
}

func main() {
	var cfg config
	flag.StringVar(&cfg.envFile, "env", ".secrets/outlook.env", "Env file with TENANT_ID/CLIENT_ID/CLIENT_SECRET")
	flag.StringVar(&cfg.tenantID, "tenant-id", os.Getenv("TENANT_ID"), "Azure tenant id")
	flag.StringVar(&cfg.clientID, "client-id", os.Getenv("CLIENT_ID"), "Azure client id")
	flag.StringVar(&cfg.clientSecret, "client-secret", os.Getenv("CLIENT_SECRET"), "Azure client secret")
	flag.StringVar(&cfg.redirectURI, "redirect-uri", os.Getenv("REDIRECT_URI"), "Redirect URI (default http://localhost)")
	flag.StringVar(&cfg.cachePath, "cache", filepath.Join(os.Getenv("HOME"), ".openclaw", "onedrive_token_cache.json"), "Token cache file")
	flag.BoolVar(&cfg.jsonOut, "json", false, "Output JSON")
	flag.Parse()

	// load env file after parsing, but before using defaults
	loadEnvFile(cfg.envFile)
	if cfg.tenantID == "" {
		cfg.tenantID = os.Getenv("TENANT_ID")
	}
	if cfg.clientID == "" {
		cfg.clientID = os.Getenv("CLIENT_ID")
	}
	if cfg.clientSecret == "" {
		cfg.clientSecret = os.Getenv("CLIENT_SECRET")
	}
	if cfg.redirectURI == "" {
		cfg.redirectURI = os.Getenv("REDIRECT_URI")
		if cfg.redirectURI == "" {
			cfg.redirectURI = "http://localhost"
		}
	}

	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	cmd := args[0]
	cmdArgs := args[1:]

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	accountPath := cfg.cachePath + ".account.json"
	authCfg := authConfig{
		tenantID:     cfg.tenantID,
		clientID:     cfg.clientID,
		clientSecret: cfg.clientSecret,
		redirectURI:  cfg.redirectURI,
		scopes:       defaultScopes,
		cachePath:    cfg.cachePath,
		accountPath:  accountPath,
	}

	at, err := acquireAccessToken(ctx, authCfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
	g := newGraphClient(at)

	switch cmd {
	case "whoami":
		handleWhoami(ctx, g, cfg.jsonOut)
	case "quota":
		handleQuota(ctx, g, cfg.jsonOut)
	case "ls":
		target := "/"
		if len(cmdArgs) >= 1 {
			target = cmdArgs[0]
		}
		handleLS(ctx, g, cfg.jsonOut, target)
	case "stat":
		if len(cmdArgs) < 1 {
			fmt.Fprintln(os.Stderr, "stat requires target")
			os.Exit(2)
		}
		handleStat(ctx, g, cfg.jsonOut, cmdArgs[0])
	case "search":
		if len(cmdArgs) < 1 {
			fmt.Fprintln(os.Stderr, "search requires query")
			os.Exit(2)
		}
		top := 50
		query := cmdArgs[0]
		if len(cmdArgs) >= 3 && cmdArgs[1] == "--top" {
			fmt.Sscanf(cmdArgs[2], "%d", &top)
		}
		handleSearch(ctx, g, cfg.jsonOut, query, top)
	case "cat":
		if len(cmdArgs) < 1 {
			fmt.Fprintln(os.Stderr, "cat requires target")
			os.Exit(2)
		}
		maxBytes := int64(200000)
		if len(cmdArgs) >= 3 && cmdArgs[1] == "--max-bytes" {
			fmt.Sscanf(cmdArgs[2], "%d", &maxBytes)
		}
		handleCat(ctx, g, cmdArgs[0], maxBytes)
	case "download":
		if len(cmdArgs) < 2 {
			fmt.Fprintln(os.Stderr, "download requires target and local path")
			os.Exit(2)
		}
		handleDownload(ctx, g, cmdArgs[0], cmdArgs[1])
	case "upload":
		if len(cmdArgs) < 2 {
			fmt.Fprintln(os.Stderr, "upload requires local_file and remote_path")
			os.Exit(2)
		}
		thresholdMB := int64(8)
		if len(cmdArgs) >= 4 && cmdArgs[2] == "--large-threshold-mb" {
			fmt.Sscanf(cmdArgs[3], "%d", &thresholdMB)
		}
		handleUpload(ctx, g, cmdArgs[0], cmdArgs[1], thresholdMB)
	case "mkdir":
		if len(cmdArgs) < 1 {
			fmt.Fprintln(os.Stderr, "mkdir requires remote_folder")
			os.Exit(2)
		}
		parents := false
		if len(cmdArgs) >= 2 && cmdArgs[1] == "--parents" {
			parents = true
		}
		handleMkdir(ctx, g, cfg.jsonOut, cmdArgs[0], parents)
	case "move":
		if len(cmdArgs) < 2 {
			fmt.Fprintln(os.Stderr, "move requires src dst")
			os.Exit(2)
		}
		handleMove(ctx, g, cfg.jsonOut, cmdArgs[0], cmdArgs[1])
	case "copy":
		if len(cmdArgs) < 2 {
			fmt.Fprintln(os.Stderr, "copy requires src dst")
			os.Exit(2)
		}
		timeoutS := int64(300)
		if len(cmdArgs) >= 4 && cmdArgs[2] == "--timeout" {
			fmt.Sscanf(cmdArgs[3], "%d", &timeoutS)
		}
		handleCopy(ctx, g, cmdArgs[0], cmdArgs[1], time.Duration(timeoutS)*time.Second)
	case "rm":
		if len(cmdArgs) < 1 {
			fmt.Fprintln(os.Stderr, "rm requires target")
			os.Exit(2)
		}
		reviewFolder := "Deleted-For-Review"
		permanent := false
		for i := 1; i < len(cmdArgs); i++ {
			if cmdArgs[i] == "--review-folder" && i+1 < len(cmdArgs) {
				reviewFolder = cmdArgs[i+1]
				i++
				continue
			}
			if cmdArgs[i] == "--permanent" {
				permanent = true
			}
		}
		handleRM(ctx, g, cfg.jsonOut, cmdArgs[0], reviewFolder, permanent)
	case "report-space":
		target := "/"
		topFiles := 20
		topExt := 15
		maxItems := 50000
		i := 0
		if len(cmdArgs) > 0 && !strings.HasPrefix(cmdArgs[0], "--") {
			target = cmdArgs[0]
			i = 1
		}
		for i < len(cmdArgs) {
			switch cmdArgs[i] {
			case "--top-files":
				i++
				topFiles = mustAtoiArg(cmdArgs, i, "--top-files")
			case "--top-ext":
				i++
				topExt = mustAtoiArg(cmdArgs, i, "--top-ext")
			case "--max-items":
				i++
				maxItems = mustAtoiArg(cmdArgs, i, "--max-items")
			default:
				fatal(fmt.Errorf("unknown flag for report-space: %s", cmdArgs[i]))
			}
			i++
		}
		handleReportSpace(ctx, g, cfg.jsonOut, target, topFiles, topExt, maxItems)
	case "organize-by-extension":
		target := "/"
		recursive := false
		execute := false
		minSize := int64(0)
		skip := []string{}
		i := 0
		if len(cmdArgs) > 0 && !strings.HasPrefix(cmdArgs[0], "--") {
			target = cmdArgs[0]
			i = 1
		}
		for i < len(cmdArgs) {
			switch cmdArgs[i] {
			case "--recursive":
				recursive = true
			case "--execute":
				execute = true
			case "--min-size":
				i++
				minSize = int64(mustAtoiArg(cmdArgs, i, "--min-size"))
			case "--skip":
				i++
				val := mustArg(cmdArgs, i, "--skip")
				for _, s := range strings.Split(val, ",") {
					s = strings.TrimSpace(strings.ToLower(s))
					if s != "" {
						skip = append(skip, s)
					}
				}
			default:
				fatal(fmt.Errorf("unknown flag for organize-by-extension: %s", cmdArgs[i]))
			}
			i++
		}
		handleOrganizeByExtension(ctx, g, cfg.jsonOut, target, recursive, execute, minSize, skip)
	default:
		fmt.Fprintln(os.Stderr, "Unknown command:", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "OneDrive Manager (Go)")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  onedrive_manager [global flags] <command> [args]")
	fmt.Fprintln(os.Stderr, "\nCommands:")
	fmt.Fprintln(os.Stderr, "  whoami | quota | ls | stat | search | cat | download | upload | mkdir | move | copy | rm | report-space | organize-by-extension")
	fmt.Fprintln(os.Stderr, "  ls/stat/cat/download also accept SharePoint/OneDrive sharing URLs")
	fmt.Fprintln(os.Stderr, "\nGlobal flags:")
	fmt.Fprintln(os.Stderr, "  --env .secrets/outlook.env  --tenant-id ... --client-id ... --client-secret ... --redirect-uri ... --cache ... --json")
}

func handleWhoami(ctx context.Context, g *graphClient, jsonOut bool) {
	var me map[string]any
	_, _, err := g.doJSON(ctx, "GET", graphBase+"/me?$select=id,displayName,userPrincipalName,mail", nil, &me, nil)
	if err != nil {
		fatal(err)
	}
	var drv map[string]any
	_, _, err = g.doJSON(ctx, "GET", graphBase+"/me/drive?$select=id,driveType,name,webUrl,quota", nil, &drv, nil)
	if err != nil {
		fatal(err)
	}
	out := map[string]any{"me": me, "drive": drv}
	if jsonOut {
		printJSON(out)
		return
	}
	fmt.Printf("User: %v <%v>\n", me["displayName"], firstNonEmpty(fmt.Sprint(me["userPrincipalName"]), fmt.Sprint(me["mail"])))
	fmt.Printf("Drive: %v (%v)\n", drv["name"], drv["driveType"])
	if q, ok := drv["quota"].(map[string]any); ok {
		used := toInt64(q["used"])
		total := toInt64(q["total"])
		fmt.Printf("Quota: %s / %s\n", fmtSize(used), fmtSize(total))
	}
}

func handleQuota(ctx context.Context, g *graphClient, jsonOut bool) {
	var drv map[string]any
	_, _, err := g.doJSON(ctx, "GET", graphBase+"/me/drive?$select=id,quota", nil, &drv, nil)
	if err != nil {
		fatal(err)
	}
	q := map[string]any{}
	if quota, ok := drv["quota"].(map[string]any); ok {
		q = map[string]any{
			"driveId":   drv["id"],
			"used":      quota["used"],
			"remaining": quota["remaining"],
			"total":     quota["total"],
			"state":     quota["state"],
		}
	}
	if jsonOut {
		printJSON(q)
		return
	}
	fmt.Println("Used:", fmtSize(toInt64(q["used"])))
	fmt.Println("Remaining:", fmtSize(toInt64(q["remaining"])))
	fmt.Println("Total:", fmtSize(toInt64(q["total"])))
	fmt.Println("State:", q["state"])
}

func handleLS(ctx context.Context, g *graphClient, jsonOut bool, target string) {
	items, err := g.listChildren(ctx, target)
	if err != nil {
		fatal(err)
	}
	if jsonOut {
		printJSON(items)
		return
	}
	for _, it := range items {
		kind := "FILE"
		if it.Folder != nil {
			kind = "DIR"
		}
		ref := "id:" + it.ID
		if strings.TrimSpace(it.ParentReference.DriveID) != "" {
			ref = "drive:" + it.ParentReference.DriveID + ":item:" + it.ID
		}
		fmt.Printf("%4s  %10s  %s  (%s)\n", kind, fmtSize(it.Size), it.Name, ref)
	}
}

func handleStat(ctx context.Context, g *graphClient, jsonOut bool, target string) {
	it, err := g.resolveItem(ctx, target)
	if err != nil {
		fatal(err)
	}
	printJSON(it)
}

func handleSearch(ctx context.Context, g *graphClient, jsonOut bool, query string, top int) {
	q := url.QueryEscape("'" + query + "'")
	urlStr := fmt.Sprintf("%s/me/drive/root/search(q=@q)?@q=%s&$top=%d", graphBase, q, clamp(top, 1, 200))
	var out struct {
		Value []driveItem `json:"value"`
	}
	_, _, err := g.doJSON(ctx, "GET", urlStr, nil, &out, nil)
	if err != nil {
		fatal(err)
	}
	if jsonOut {
		printJSON(out.Value)
		return
	}
	for _, it := range out.Value {
		kind := "FILE"
		if it.Folder != nil {
			kind = "DIR"
		}
		fmt.Printf("%4s  %s  id:%s\n", kind, it.Name, it.ID)
	}
}

func handleCat(ctx context.Context, g *graphClient, target string, maxBytes int64) {
	rt, err := g.resolveTarget(ctx, target)
	if err != nil {
		fatal(err)
	}
	if rt.Item.File == nil {
		fatal(fmt.Errorf("only files can be read"))
	}
	urlStr := rt.ContentURL
	req, _ := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	req.Header.Set("Authorization", "Bearer "+g.accessToken)
	resp, err := g.http.Do(req)
	if err != nil {
		fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		fatal(fmt.Errorf("read failed %d: %s", resp.StatusCode, truncate(string(b), 800)))
	}
	buf := make([]byte, 0, maxBytes)
	chunk := make([]byte, 64*1024)
	var total int64
	for {
		n, err := resp.Body.Read(chunk)
		if n > 0 {
			if total+int64(n) > maxBytes {
				n = int(maxBytes - total)
			}
			buf = append(buf, chunk[:n]...)
			total += int64(n)
			if total >= maxBytes {
				break
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			fatal(err)
		}
	}
	os.Stdout.Write(buf)
}

func handleDownload(ctx context.Context, g *graphClient, target, localPath string) {
	rt, err := g.resolveTarget(ctx, target)
	if err != nil {
		fatal(err)
	}
	if rt.Item.File == nil {
		fatal(fmt.Errorf("only files can be downloaded"))
	}
	urlStr := rt.ContentURL
	req, _ := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	req.Header.Set("Authorization", "Bearer "+g.accessToken)
	resp, err := g.http.Do(req)
	if err != nil {
		fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		fatal(fmt.Errorf("download failed %d: %s", resp.StatusCode, truncate(string(b), 800)))
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		fatal(err)
	}
	f, err := os.Create(localPath)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		fatal(err)
	}
	fmt.Println("Downloaded to", mustAbs(localPath))
}

func handleUpload(ctx context.Context, g *graphClient, localFile, remotePath string, largeThresholdMB int64) {
	st, err := os.Stat(localFile)
	if err != nil || st.IsDir() {
		fatal(fmt.Errorf("local file not found: %s", localFile))
	}
	threshold := largeThresholdMB * 1024 * 1024
	if st.Size() <= threshold {
		// small upload
		b, err := os.ReadFile(localFile)
		if err != nil {
			fatal(err)
		}
		enc := encodePath(remotePath)
		urlStr := graphBase + "/me/drive/root:" + enc + ":/content"
		req, _ := http.NewRequestWithContext(ctx, "PUT", urlStr, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+g.accessToken)
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := g.http.Do(req)
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 400 {
			fatal(fmt.Errorf("upload failed %d: %s", resp.StatusCode, truncate(string(body), 800)))
		}
		fmt.Println("Uploaded")
		return
	}

	// large upload session
	remotePath = strings.TrimSpace(remotePath)
	if !strings.HasPrefix(remotePath, "/") {
		remotePath = "/" + remotePath
	}
	parentDir := path.Dir(remotePath)
	fileName := path.Base(remotePath)
	if fileName == "." || fileName == "/" || fileName == "" {
		fatal(fmt.Errorf("remote path must include file name"))
	}

	parentItem, err := g.itemByPath(ctx, parentDir)
	if err != nil {
		fatal(err)
	}
	sessURL := fmt.Sprintf("%s/me/drive/items/%s:/%s:/createUploadSession", graphBase, url.PathEscape(parentItem.ID), url.PathEscape(fileName))
	var sess struct {
		UploadURL string `json:"uploadUrl"`
	}
	_, _, err = g.doJSON(ctx, "POST", sessURL, map[string]any{"item": map[string]any{"@microsoft.graph.conflictBehavior": "replace", "name": fileName}}, &sess, nil)
	if err != nil {
		fatal(err)
	}
	if sess.UploadURL == "" {
		fatal(fmt.Errorf("no uploadUrl in upload session"))
	}

	// chunk upload
	chunkSize := int64(10 * 320 * 1024)
	f, err := os.Open(localFile)
	if err != nil {
		fatal(err)
	}
	defer f.Close()

	size := st.Size()
	var sent int64
	buf := make([]byte, chunkSize)
	for sent < size {
		start := sent
		end := start + chunkSize
		if end > size {
			end = size
		}
		toRead := end - start
		n, err := io.ReadFull(f, buf[:toRead])
		if err != nil && err != io.ErrUnexpectedEOF {
			fatal(err)
		}
		part := buf[:n]

		req, _ := http.NewRequestWithContext(ctx, "PUT", sess.UploadURL, bytes.NewReader(part))
		req.Header.Set("Content-Length", fmt.Sprintf("%d", len(part)))
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, start+int64(len(part))-1, size))
		resp, err := g.http.Do(req)
		if err != nil {
			fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 200 || resp.StatusCode == 201 {
			fmt.Println("Uploaded")
			return
		}
		if resp.StatusCode == 202 {
			sent = start + int64(len(part))
			continue
		}
		fatal(fmt.Errorf("chunk upload failed %d: %s", resp.StatusCode, truncate(string(body), 800)))
	}
	fmt.Println("Upload session finished")
}

func handleMkdir(ctx context.Context, g *graphClient, jsonOut bool, folderPath string, parents bool) {
	folderPath = strings.TrimSpace(folderPath)
	folderPath = strings.TrimSuffix(folderPath, "/")
	if folderPath == "" {
		fatal(fmt.Errorf("invalid folder path"))
	}
	if !strings.HasPrefix(folderPath, "/") {
		folderPath = "/" + folderPath
	}

	if parents {
		// ensure each segment exists
		segments := strings.Split(strings.TrimPrefix(folderPath, "/"), "/")
		cur := "/"
		for _, seg := range segments {
			if seg == "" {
				continue
			}
			cur = path.Join(cur, seg)
			// stat; if missing, create
			_, err := g.itemByPath(ctx, cur)
			if err == nil {
				continue
			}
			parent := path.Dir(cur)
			parentItem, err2 := g.itemByPath(ctx, parent)
			if err2 != nil {
				fatal(err2)
			}
			name := path.Base(cur)
			createFolder(ctx, g, parentItem.ID, name)
		}
		out, err := g.itemByPath(ctx, folderPath)
		if err != nil {
			fatal(err)
		}
		printJSON(out)
		return
	}

	parent := path.Dir(folderPath)
	name := path.Base(folderPath)
	parentItem, err := g.itemByPath(ctx, parent)
	if err != nil {
		fatal(err)
	}
	out, err := createFolder(ctx, g, parentItem.ID, name)
	if err != nil {
		fatal(err)
	}
	printJSON(out)
}

func createFolder(ctx context.Context, g *graphClient, parentID, name string) (driveItem, error) {
	body := map[string]any{"name": name, "folder": map[string]any{}, "@microsoft.graph.conflictBehavior": "fail"}
	var out driveItem
	_, _, err := g.doJSON(ctx, "POST", graphBase+"/me/drive/items/"+url.PathEscape(parentID)+"/children", body, &out, nil)
	return out, err
}

func handleMove(ctx context.Context, g *graphClient, jsonOut bool, src, dst string) {
	item, err := g.resolveItem(ctx, src)
	if err != nil {
		fatal(err)
	}

	// dst can be folder or full path
	var parentID string
	newName := item.Name
	if it, err := g.resolveItem(ctx, dst); err == nil && it.Folder != nil {
		parentID = it.ID
	} else {
		dst = strings.TrimSuffix(dst, "/")
		parentPath := path.Dir(dst)
		name := path.Base(dst)
		if name != "." && name != "" && name != "/" {
			newName = name
		}
		parentItem, err := g.itemByPath(ctx, parentPath)
		if err != nil {
			fatal(err)
		}
		if parentItem.Folder == nil {
			fatal(fmt.Errorf("destination parent is not a folder"))
		}
		parentID = parentItem.ID
	}

	body := map[string]any{"parentReference": map[string]any{"id": parentID}, "name": newName}
	var out driveItem
	_, _, err = g.doJSON(ctx, "PATCH", graphBase+"/me/drive/items/"+url.PathEscape(item.ID), body, &out, nil)
	if err != nil {
		fatal(err)
	}
	printJSON(out)
}

func handleCopy(ctx context.Context, g *graphClient, src, dst string, timeout time.Duration) {
	item, err := g.resolveItem(ctx, src)
	if err != nil {
		fatal(err)
	}

	// determine destination parent + name
	var parentID string
	newName := item.Name
	if it, err := g.resolveItem(ctx, dst); err == nil && it.Folder != nil {
		parentID = it.ID
	} else {
		dst = strings.TrimSuffix(dst, "/")
		parentPath := path.Dir(dst)
		name := path.Base(dst)
		if name != "." && name != "" && name != "/" {
			newName = name
		}
		parentItem, err := g.itemByPath(ctx, parentPath)
		if err != nil {
			fatal(err)
		}
		parentID = parentItem.ID
	}

	body := map[string]any{"parentReference": map[string]any{"id": parentID}, "name": newName}
	urlStr := graphBase + "/me/drive/items/" + url.PathEscape(item.ID) + "/copy"
	req, _ := http.NewRequestWithContext(ctx, "POST", urlStr, strings.NewReader(mustJSON(body)))
	req.Header.Set("Authorization", "Bearer "+g.accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.http.Do(req)
	if err != nil {
		fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 202 && resp.StatusCode != 200 && resp.StatusCode != 201 {
		fatal(fmt.Errorf("copy failed: %d", resp.StatusCode))
	}
	monitor := resp.Header.Get("Location")
	if monitor == "" {
		fmt.Println("Copy request accepted")
		return
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, "GET", monitor, nil)
		resp, err := g.http.Do(req)
		if err != nil {
			fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 202 {
			time.Sleep(2 * time.Second)
			continue
		}
		if resp.StatusCode >= 400 {
			fatal(fmt.Errorf("copy monitor error %d: %s", resp.StatusCode, truncate(string(b), 600)))
		}
		// best-effort parse
		var st map[string]any
		_ = json.Unmarshal(b, &st)
		if s, _ := st["status"].(string); s == "completed" {
			fmt.Println("Copy completed")
			return
		}
		if s, _ := st["status"].(string); s == "failed" {
			fatal(fmt.Errorf("copy failed: %s", truncate(string(b), 600)))
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Println("Copy still running (monitor timeout reached)")
}

func handleRM(ctx context.Context, g *graphClient, jsonOut bool, target, reviewFolder string, permanent bool) {
	it, err := g.resolveItem(ctx, target)
	if err != nil {
		fatal(err)
	}
	if permanent {
		urlStr := graphBase + "/me/drive/items/" + url.PathEscape(it.ID) + "/permanentDelete"
		_, _, err := g.doJSON(ctx, "POST", urlStr, map[string]any{}, nil, nil)
		if err != nil {
			// fallback: DELETE
			_, _, err2 := g.doJSON(ctx, "DELETE", graphBase+"/me/drive/items/"+url.PathEscape(it.ID), nil, nil, nil)
			if err2 != nil {
				fatal(err)
			}
		}
		fmt.Println("Permanently deleted")
		return
	}

	// safe-delete move to sibling review folder
	full, err := g.itemByID(ctx, it.ID)
	if err != nil {
		fatal(err)
	}
	parentID := full.ParentReference.ID
	if parentID == "" {
		fatal(fmt.Errorf("cannot move item without parent reference"))
	}
	if full.Name == reviewFolder && full.Folder != nil {
		fatal(fmt.Errorf("target is already the review folder"))
	}

	review, err := getOrCreateChildFolder(ctx, g, parentID, reviewFolder)
	if err != nil {
		fatal(err)
	}

	newName := full.Name
	body := map[string]any{"parentReference": map[string]any{"id": review.ID}, "name": newName}
	var out driveItem
	_, raw, err := g.doJSON(ctx, "PATCH", graphBase+"/me/drive/items/"+url.PathEscape(full.ID), body, &out, nil)
	if err != nil {
		// collision? append timestamp
		if strings.Contains(string(raw), "nameAlreadyExists") || strings.Contains(string(raw), "already exists") {
			ts := time.Now().Unix()
			ext := filepath.Ext(newName)
			stem := strings.TrimSuffix(newName, ext)
			alt := fmt.Sprintf("%s__review_%d%s", stem, ts, ext)
			body["name"] = alt
			_, _, err2 := g.doJSON(ctx, "PATCH", graphBase+"/me/drive/items/"+url.PathEscape(full.ID), body, &out, nil)
			if err2 != nil {
				fatal(err2)
			}
			printJSON(out)
			return
		}
		fatal(err)
	}
	printJSON(out)
}

type sizeByExt struct {
	Ext   string `json:"ext"`
	Bytes int64  `json:"bytes"`
}

type largestFile struct {
	Bytes int64  `json:"bytes"`
	Link  string `json:"link"`
}

type spaceReport struct {
	Target       string        `json:"target"`
	TotalItems   int           `json:"totalItems"`
	TotalFolders int           `json:"totalFolders"`
	TotalFiles   int           `json:"totalFiles"`
	TotalBytes   int64         `json:"totalBytes"`
	EmptyFolders int           `json:"emptyFolders"`
	TopExt       []sizeByExt   `json:"topExtensions"`
	LargestFiles []largestFile `json:"largestFiles"`
}

type organizePlanItem struct {
	ItemID       string `json:"itemId"`
	ItemName     string `json:"itemName"`
	Size         int64  `json:"size"`
	FromFolderID string `json:"fromFolderId"`
	ToFolderPath string `json:"toFolderPath"`
}

type organizeResult struct {
	Target       string             `json:"target"`
	PlannedMoves int                `json:"plannedMoves"`
	Executed     bool               `json:"executed"`
	Moved        int                `json:"moved"`
	Plan         []organizePlanItem `json:"plan"`
}

func handleReportSpace(ctx context.Context, g *graphClient, jsonOut bool, target string, topFiles, topExt, maxItems int) {
	report, err := buildSpaceReport(ctx, g, target, topFiles, topExt, maxItems)
	if err != nil {
		fatal(err)
	}
	if jsonOut {
		printJSON(report)
		return
	}

	fmt.Printf("Target: %s\n", report.Target)
	fmt.Printf("Items: %d | Folders: %d | Files: %d\n", report.TotalItems, report.TotalFolders, report.TotalFiles)
	fmt.Printf("Used by files: %s\n", fmtSize(report.TotalBytes))
	fmt.Printf("Empty folders: %d\n", report.EmptyFolders)
	fmt.Println("\nTop extensions by size:")
	for _, e := range report.TopExt {
		fmt.Printf("  %-12s %s\n", e.Ext, fmtSize(e.Bytes))
	}
	fmt.Println("\nLargest files:")
	for _, lf := range report.LargestFiles {
		fmt.Printf("  %10s  %s\n", fmtSize(lf.Bytes), lf.Link)
	}
}

func buildSpaceReport(ctx context.Context, g *graphClient, target string, topFiles, topExt, maxItems int) (spaceReport, error) {
	root, err := g.resolveItem(ctx, target)
	if err != nil {
		return spaceReport{}, err
	}
	if root.Folder == nil {
		return spaceReport{}, fmt.Errorf("report-space target must be folder")
	}

	totalItems := 0
	totalFiles := 0
	totalFolders := 0
	emptyFolders := 0
	totalBytes := int64(0)
	extBytes := map[string]int64{}
	largest := []largestFile{}

	queue := []driveItem{root}
	for len(queue) > 0 {
		folder := queue[0]
		queue = queue[1:]
		totalFolders++

		children, err := g.listChildrenByID(ctx, folder.ID)
		if err != nil {
			return spaceReport{}, err
		}
		if len(children) == 0 {
			emptyFolders++
		}

		for _, ch := range children {
			totalItems++
			if totalItems > maxItems {
				return spaceReport{}, fmt.Errorf("aborted: exceeded max-items=%d", maxItems)
			}

			if ch.Folder != nil {
				queue = append(queue, ch)
				continue
			}

			totalFiles++
			totalBytes += ch.Size
			ext := strings.ToLower(filepath.Ext(ch.Name))
			if ext == "" {
				ext = "<no-ext>"
			}
			extBytes[ext] += ch.Size

			link := strings.TrimSpace(ch.WebURL)
			if link == "" {
				link = "id:" + ch.ID
			}
			largest = append(largest, largestFile{Bytes: ch.Size, Link: link})
		}
	}

	orderedExt := make([]sizeByExt, 0, len(extBytes))
	for ext, b := range extBytes {
		orderedExt = append(orderedExt, sizeByExt{Ext: ext, Bytes: b})
	}
	sort.Slice(orderedExt, func(i, j int) bool { return orderedExt[i].Bytes > orderedExt[j].Bytes })
	if topExt > 0 && len(orderedExt) > topExt {
		orderedExt = orderedExt[:topExt]
	}

	sort.Slice(largest, func(i, j int) bool { return largest[i].Bytes > largest[j].Bytes })
	if topFiles > 0 && len(largest) > topFiles {
		largest = largest[:topFiles]
	}

	return spaceReport{
		Target:       target,
		TotalItems:   totalItems,
		TotalFolders: totalFolders,
		TotalFiles:   totalFiles,
		TotalBytes:   totalBytes,
		EmptyFolders: emptyFolders,
		TopExt:       orderedExt,
		LargestFiles: largest,
	}, nil
}

func handleOrganizeByExtension(ctx context.Context, g *graphClient, jsonOut bool, target string, recursive, execute bool, minSize int64, skip []string) {
	result, err := organizeByExtension(ctx, g, target, recursive, execute, minSize, skip)
	if err != nil {
		fatal(err)
	}

	if jsonOut {
		printJSON(result)
		return
	}

	mode := "DRY-RUN"
	if result.Executed {
		mode = "EXECUTED"
	}
	fmt.Printf("Mode: %s\n", mode)
	fmt.Printf("Planned moves: %d\n", result.PlannedMoves)
	fmt.Printf("Moved: %d\n", result.Moved)
	fmt.Println("Sample plan:")
	limit := len(result.Plan)
	if limit > 20 {
		limit = 20
	}
	for i := 0; i < limit; i++ {
		p := result.Plan[i]
		fmt.Printf("  %s -> %s\n", p.ItemName, p.ToFolderPath)
	}
}

func organizeByExtension(ctx context.Context, g *graphClient, target string, recursive, execute bool, minSize int64, skip []string) (organizeResult, error) {
	root, err := g.resolveItem(ctx, target)
	if err != nil {
		return organizeResult{}, err
	}
	if root.Folder == nil {
		return organizeResult{}, fmt.Errorf("organize target must be folder")
	}

	skipSet := map[string]bool{}
	for _, s := range skip {
		s = strings.TrimSpace(strings.ToLower(s))
		if s != "" {
			skipSet[s] = true
		}
	}

	type folderNode struct {
		item     driveItem
		fullPath string
	}
	queue := []folderNode{{item: root, fullPath: normalizeFolderPath(target, root)}}
	plan := make([]organizePlanItem, 0)

	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]

		children, err := g.listChildrenByID(ctx, n.item.ID)
		if err != nil {
			return organizeResult{}, err
		}

		for _, ch := range children {
			if ch.Folder != nil {
				if recursive {
					queue = append(queue, folderNode{item: ch, fullPath: path.Join(n.fullPath, ch.Name)})
				}
				continue
			}

			if ch.Size < minSize {
				continue
			}

			ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(ch.Name), "."))
			extKey := ext
			if extKey == "" {
				extKey = "no_extension"
			}
			if skipSet[extKey] {
				continue
			}

			toFolderPath := path.Join(n.fullPath, strings.ToUpper(extKey))
			plan = append(plan, organizePlanItem{
				ItemID:       ch.ID,
				ItemName:     ch.Name,
				Size:         ch.Size,
				FromFolderID: n.item.ID,
				ToFolderPath: toFolderPath,
			})
		}
	}

	moved := 0
	if execute {
		for _, p := range plan {
			dest, err := g.ensureFolderPath(ctx, p.ToFolderPath)
			if err != nil {
				return organizeResult{}, err
			}
			body := map[string]any{"parentReference": map[string]any{"id": dest.ID}, "name": p.ItemName}
			if _, _, err := g.doJSON(ctx, "PATCH", graphBase+"/me/drive/items/"+url.PathEscape(p.ItemID), body, nil, nil); err != nil {
				return organizeResult{}, err
			}
			moved++
		}
	}

	planned := len(plan)
	if len(plan) > 2000 {
		plan = plan[:2000]
	}

	return organizeResult{
		Target:       target,
		PlannedMoves: planned,
		Executed:     execute,
		Moved:        moved,
		Plan:         plan,
	}, nil
}

func normalizeFolderPath(target string, it driveItem) string {
	t := strings.TrimSpace(target)
	if t != "" && !strings.HasPrefix(t, "id:") {
		if !strings.HasPrefix(t, "/") {
			t = "/" + t
		}
		return path.Clean(t)
	}

	parent := strings.TrimSpace(it.ParentReference.Path)
	parent = strings.TrimPrefix(parent, "/drive/root:")
	if parent == "" {
		if strings.EqualFold(it.Name, "root") || strings.TrimSpace(it.Name) == "" {
			return "/"
		}
		return path.Clean("/" + it.Name)
	}
	if strings.TrimSpace(it.Name) == "" {
		return path.Clean(parent)
	}
	return path.Clean(path.Join(parent, it.Name))
}

func getOrCreateChildFolder(ctx context.Context, g *graphClient, parentID, name string) (driveItem, error) {
	// list children and find folder
	urlStr := graphBase + "/me/drive/items/" + url.PathEscape(parentID) + "/children?$top=200&$select=id,name,folder"
	for urlStr != "" {
		var page listChildrenResp
		_, _, err := g.doJSON(ctx, "GET", urlStr, nil, &page, nil)
		if err != nil {
			return driveItem{}, err
		}
		for _, it := range page.Value {
			if it.Name == name && it.Folder != nil {
				return it, nil
			}
		}
		urlStr = page.NextLink
	}
	return createFolder(ctx, g, parentID, name)
}

// --------------------------
// Utilities
// --------------------------

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ERROR:", err)
	os.Exit(1)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" && v != "<nil>" {
			return v
		}
	}
	return ""
}

func toInt64(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case int:
		return int64(t)
	case json.Number:
		i, _ := t.Int64()
		return i
	case string:
		var out int64
		fmt.Sscanf(t, "%d", &out)
		return out
	default:
		return 0
	}
}

func fmtSize(n int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v := float64(n)
	for i, u := range units {
		if v < 1024.0 || i == len(units)-1 {
			if u == "B" {
				return fmt.Sprintf("%d %s", int64(v), u)
			}
			return fmt.Sprintf("%.2f %s", v, u)
		}
		v /= 1024.0
	}
	return fmt.Sprintf("%d B", n)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func mustArg(args []string, idx int, flagName string) string {
	if idx >= len(args) {
		fatal(fmt.Errorf("%s requires a value", flagName))
	}
	return args[idx]
}

func mustAtoiArg(args []string, idx int, flagName string) int {
	raw := mustArg(args, idx, flagName)
	n, err := strconv.Atoi(raw)
	if err != nil {
		fatal(fmt.Errorf("invalid integer for %s: %s", flagName, raw))
	}
	return n
}

func mustAbs(p string) string {
	ap, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return ap
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
