// openproject_ops — OpenProject API v3 CLI tool.
//
// Credentials are loaded from an env file (.secrets/openproject.env by default)
// or from environment variables. Env vars always take precedence over the file.
//
// Required env: OPENPROJECT_BASE_URL, OPENPROJECT_TOKEN
// Optional env: OPENPROJECT_API_PATH (default /api/v3), OPENPROJECT_ENV_FILE
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	version                 = "2.2.1"
	defaultAPIPath          = "/api/v3"
	defaultTimeout          = 60 * time.Second
	defaultPageSize         = 200
	maxPageSize             = 1000
	maxNotableTasks         = 15
	defaultTasksLimit       = 50
	defaultParallel         = 4
	maxParallel             = 16
	defaultConfigFileName   = "openproject.config.json"
	defaultConfigExampleURL = "https://your-openproject.example.com/"
	endOfDayNano            = 999_999_999
	envFileSearchDepth      = 4 // how many parent dirs to walk looking for .secrets/
)

// ---------------------------------------------------------------------------
// Config + env-file loader
// ---------------------------------------------------------------------------

type configFile struct {
	BaseURL  string `json:"base_url"`
	Token    string `json:"token"`
	APIPath  string `json:"api_path"`
	Parallel int    `json:"parallel"`
	PageSize int    `json:"page_size"`
}

func setEnvIfEmpty(k, v string) {
	if strings.TrimSpace(v) == "" {
		return
	}
	if os.Getenv(k) == "" {
		_ = os.Setenv(k, strings.TrimSpace(v))
	}
}

func loadJSONConfigFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cfg configFile
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	setEnvIfEmpty("OPENPROJECT_BASE_URL", cfg.BaseURL)
	setEnvIfEmpty("OPENPROJECT_TOKEN", cfg.Token)
	setEnvIfEmpty("OPENPROJECT_API_PATH", cfg.APIPath)
	if cfg.Parallel > 0 {
		setEnvIfEmpty("OPENPROJECT_PARALLEL", strconv.Itoa(cfg.Parallel))
	}
	if cfg.PageSize > 0 {
		setEnvIfEmpty("OPENPROJECT_PAGE_SIZE", strconv.Itoa(cfg.PageSize))
	}
	return nil
}

// loadEnvFile reads a KEY=VALUE file and sets env vars that are not already set.
// Supports # comments, inline comments, export prefix, and quoted values.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)

		eq := strings.IndexByte(line, '=')
		if eq < 1 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])

		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if idx := strings.Index(val, " #"); idx >= 0 {
			val = strings.TrimSpace(val[:idx])
		}
		setEnvIfEmpty(key, val)
	}
	return sc.Err()
}

func findNearestSecretsFile(fileName string) string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < envFileSearchDepth; i++ {
		candidate := filepath.Join(dir, ".secrets", fileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// loadOpenProjectConfig loads (in order):
// 1) OPENPROJECT_CONFIG_FILE or nearest .secrets/openproject.config.json
// 2) OPENPROJECT_ENV_FILE or nearest .secrets/openproject.env
// Real env vars always win because files only set missing values.
func loadOpenProjectConfig() {
	if p := strings.TrimSpace(os.Getenv("OPENPROJECT_CONFIG_FILE")); p != "" {
		_ = loadJSONConfigFile(p)
	} else if p := findNearestSecretsFile(defaultConfigFileName); p != "" {
		_ = loadJSONConfigFile(p)
	}

	if p := strings.TrimSpace(os.Getenv("OPENPROJECT_ENV_FILE")); p != "" {
		_ = loadEnvFile(p)
	} else if p := findNearestSecretsFile("openproject.env"); p != "" {
		_ = loadEnvFile(p)
	}
}

func envOrDie(name, fallback string) string {
	v := os.Getenv(name)
	if v == "" {
		v = fallback
	}
	if v == "" {
		fatalf("Missing required environment variable: %s", name)
	}
	return v
}

func envInt(name string, fallback, minVal, maxVal int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if n < minVal {
		return minVal
	}
	if n > maxVal {
		return maxVal
	}
	return n
}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

// Client holds connection info for an OpenProject instance.
type Client struct {
	baseURL  string
	apiPath  string
	auth     string // Basic auth header value
	http     *http.Client
	parallel int
	pageSize int
}

// NewClient builds a Client from base URL, API token, and API path.
func NewClient(baseURL, token, apiPath string, parallel int, pageSize int) *Client {
	if !strings.HasPrefix(apiPath, "/") {
		apiPath = "/" + apiPath
	}
	if parallel < 1 {
		parallel = 1
	}
	if parallel > maxParallel {
		parallel = maxParallel
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		apiPath:  apiPath,
		auth:     "Basic " + base64.StdEncoding.EncodeToString([]byte("apikey:"+token)),
		http:     &http.Client{Timeout: defaultTimeout, Transport: transport},
		parallel: parallel,
		pageSize: pageSize,
	}
}

// apiURL builds a full URL for an API path (relative to apiPath).
func (c *Client) apiURL(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.baseURL + c.apiPath + path
}

// do is the single HTTP executor shared by all request helpers.
func (c *Client) do(ctx context.Context, method, fullURL string, body any) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal body: %w", err)
		}
		rdr = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, rdr)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("HTTP %s %s: %w", method, fullURL, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, nil
}

// request calls the API and returns parsed JSON (or an error for 4xx/5xx).
func (c *Client) request(ctx context.Context, method, path string, query map[string]any, body any) (map[string]any, error) {
	u, err := url.Parse(c.apiURL(path))
	if err != nil {
		return nil, err
	}
	if len(query) > 0 {
		q := u.Query()
		for k, v := range query {
			switch t := v.(type) {
			case string:
				q.Set(k, t)
			default:
				b, _ := json.Marshal(v)
				q.Set(k, string(b))
			}
		}
		u.RawQuery = q.Encode()
	}

	raw, code, err := c.do(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		return nil, fmt.Errorf("HTTP %d %s %s\n%s", code, method, path, truncate(string(raw), 500))
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return out, nil
}

// Convenience wrappers.
func (c *Client) get(path string, q map[string]any) (map[string]any, error) {
	return c.request(context.Background(), "GET", path, q, nil)
}

func (c *Client) getCtx(ctx context.Context, path string, q map[string]any) (map[string]any, error) {
	return c.request(ctx, "GET", path, q, nil)
}
func (c *Client) post(path string, body any) (map[string]any, error) {
	return c.request(context.Background(), "POST", path, nil, body)
}
func (c *Client) patch(path string, body any) (map[string]any, error) {
	return c.request(context.Background(), "PATCH", path, nil, body)
}
func (c *Client) delete(path string) (map[string]any, error) {
	return c.request(context.Background(), "DELETE", path, nil, nil)
}

// fetchAllPages collects all elements across paginated responses.
// OpenProject offsets are 1-based page indexes (not item indexes).
func (c *Client) fetchAllPages(path string, query map[string]any) ([]map[string]any, error) {
	if query == nil {
		query = map[string]any{}
	}
	pageSize := intVal(query["pageSize"])
	if pageSize <= 0 {
		pageSize = c.pageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	baseQ := cloneMap(query)
	baseQ["pageSize"] = pageSize
	baseQ["offset"] = 1

	first, err := c.get(path, baseQ)
	if err != nil {
		return nil, err
	}
	firstItems := flattenElements(first)
	total := intVal(first["total"])
	if total <= len(firstItems) || total <= pageSize {
		return firstItems, nil
	}

	totalPages := total / pageSize
	if total%pageSize != 0 {
		totalPages++
	}
	if totalPages <= 1 {
		return firstItems, nil
	}

	workers := c.parallel
	if workers > totalPages-1 {
		workers = totalPages - 1
	}
	if workers < 1 {
		workers = 1
	}

	type pageResult struct {
		page  int
		items []map[string]any
		err   error
	}
	jobs := make(chan int)
	results := make(chan pageResult, totalPages-1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for page := range jobs {
				q := cloneMap(baseQ)
				q["offset"] = page
				data, err := c.getCtx(ctx, path, q)
				if err != nil {
					results <- pageResult{page: page, err: err}
					continue
				}
				results <- pageResult{page: page, items: flattenElements(data)}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for page := 2; page <= totalPages; page++ {
			select {
			case <-ctx.Done():
				return
			case jobs <- page:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	pages := make(map[int][]map[string]any, totalPages)
	pages[1] = firstItems
	var firstErr error
	for res := range results {
		if res.err != nil {
			if firstErr == nil {
				firstErr = res.err
				cancel()
			}
			continue
		}
		pages[res.page] = res.items
	}
	if firstErr != nil {
		return nil, firstErr
	}

	all := make([]map[string]any, 0, total)
	for page := 1; page <= totalPages; page++ {
		all = append(all, pages[page]...)
	}
	if len(all) > total {
		all = all[:total]
	}
	return all, nil
}

// openAPISpec fetches the OpenAPI spec with compatibility fallbacks.
//
// OpenProject docs expose spec endpoints as /docs/api/v3/spec.json (and spec.yml),
// while many instances expose /api/v3/openapi.json directly.
func (c *Client) openAPISpec() (map[string]any, error) {
	candidates := []string{
		c.baseURL + "/api/v3/openapi.json",
		c.baseURL + "/api/v3/spec.json",
		c.baseURL + "/docs/api/v3/spec.json",
	}

	var lastErr error
	for _, u := range candidates {
		raw, code, err := c.do(context.Background(), "GET", u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		if code >= 400 {
			lastErr = fmt.Errorf("HTTP %d fetching OpenAPI spec from %s", code, u)
			continue
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			lastErr = fmt.Errorf("decode OpenAPI spec from %s: %w", u, err)
			continue
		}
		return out, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("unable to fetch OpenAPI spec")
	}
	return nil, lastErr
}

// ---------------------------------------------------------------------------
// HAL helpers
// ---------------------------------------------------------------------------

func flattenElements(data map[string]any) []map[string]any {
	emb, _ := data["_embedded"].(map[string]any)
	elems, _ := emb["elements"].([]any)
	out := make([]map[string]any, 0, len(elems))
	for _, e := range elems {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func linkHref(m map[string]any, key string) string {
	links, _ := m["_links"].(map[string]any)
	obj, _ := links[key].(map[string]any)
	return str(obj["href"])
}

func linkTitle(m map[string]any, key, def string) string {
	links, _ := m["_links"].(map[string]any)
	obj, _ := links[key].(map[string]any)
	if t := str(obj["title"]); t != "" {
		return t
	}
	return def
}

func toRelativeAPIPath(c *Client, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		u, err := url.Parse(href)
		if err == nil {
			href = u.Path
		}
	}
	if strings.HasPrefix(href, c.apiPath+"/") {
		href = strings.TrimPrefix(href, c.apiPath)
	}
	if href == c.apiPath {
		return "/"
	}
	if !strings.HasPrefix(href, "/") {
		href = "/" + href
	}
	return href
}

// ---------------------------------------------------------------------------
// Type-conversion helpers
// ---------------------------------------------------------------------------

func str(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func intVal(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		n, _ := strconv.Atoi(t)
		return n
	default:
		return 0
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// ---------------------------------------------------------------------------
// Date / time helpers
// ---------------------------------------------------------------------------

func parseISO(ts string) (time.Time, error) {
	if ts == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	// time.RFC3339 handles both "Z" and "+00:00" suffixes.
	return time.Parse(time.RFC3339, ts)
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func endOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, endOfDayNano, time.UTC)
}

func parseDateRange(from, to string) (time.Time, time.Time, error) {
	d1, err := time.Parse("2006-01-02", from)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid --from date: %w", err)
	}
	d2, err := time.Parse("2006-01-02", to)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid --to date: %w", err)
	}
	if d2.Before(d1) {
		return time.Time{}, time.Time{}, fmt.Errorf("--to (%s) must be on or after --from (%s)", to, from)
	}
	return startOfDay(d1), endOfDay(d2), nil
}

func mondayOfWeek() time.Time {
	now := time.Now()
	wd := int(now.Weekday())
	if wd == 0 {
		wd = 7
	}
	return startOfDay(now.AddDate(0, 0, -(wd - 1)))
}

// ---------------------------------------------------------------------------
// Project helpers
// ---------------------------------------------------------------------------

// resolveProject returns the numeric project ID string given an ID or identifier.
func resolveProject(c *Client, ref string) (string, error) {
	if _, err := strconv.Atoi(ref); err == nil {
		return ref, nil
	}
	p, err := c.get("/projects/"+url.PathEscape(ref), nil)
	if err != nil {
		return "", fmt.Errorf("resolve project %q: %w", ref, err)
	}
	id := str(p["id"])
	if id == "" || id == "0" {
		return "", fmt.Errorf("could not resolve project: %s", ref)
	}
	return id, nil
}

// ---------------------------------------------------------------------------
// User helpers
// ---------------------------------------------------------------------------

// resolveUserHref finds a user by numeric ID or name search and returns the
// HAL self-href (relative, e.g. /api/v3/users/42).
func resolveUserHref(c *Client, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	// Numeric ID → direct lookup.
	if _, err := strconv.Atoi(ref); err == nil {
		u, err := c.get("/users/"+ref, nil)
		if err != nil {
			return "", fmt.Errorf("lookup user %s: %w", ref, err)
		}
		return linkHref(u, "self"), nil
	}
	// Name search.
	filters := []any{map[string]any{"name": map[string]any{"operator": "~", "values": []string{ref}}}}
	data, err := c.get("/users", map[string]any{"filters": filters, "pageSize": 20})
	if err != nil {
		return "", err
	}
	elems := flattenElements(data)
	if len(elems) == 0 {
		return "", nil
	}
	needle := strings.ToLower(strings.TrimSpace(ref))
	best := elems[0]
	for _, u := range elems {
		if strings.ToLower(str(u["name"])) == needle {
			best = u
			break
		}
	}
	return linkHref(best, "self"), nil
}

// ---------------------------------------------------------------------------
// Task-type helper
// ---------------------------------------------------------------------------

func resolveTaskTypeHref(c *Client, projectID string) (string, error) {
	data, err := c.get("/projects/"+projectID+"/types", map[string]any{"pageSize": 100})
	if err != nil {
		return "", err
	}
	types := flattenElements(data)
	if len(types) == 0 {
		return "", fmt.Errorf("no work-package types found for project %s", projectID)
	}
	best := types[0]
	for _, t := range types {
		if strings.EqualFold(str(t["name"]), "task") {
			best = t
			break
		}
	}
	href := linkHref(best, "self")
	if href == "" {
		return "", fmt.Errorf("could not resolve task type href")
	}
	return href, nil
}

// ---------------------------------------------------------------------------
// Activity / report types
// ---------------------------------------------------------------------------

// MemberStats tracks per-member work-package counts in a date range.
type MemberStats struct {
	Created int
	Updated int
	Closed  int
}

type CXTaskRow struct {
	ID        string `json:"id"`
	Subject   string `json:"subject"`
	Status    string `json:"status"`
	Assignee  string `json:"assignee"`
	UpdatedAt string `json:"updatedAt"`
	CreatedAt string `json:"createdAt"`
}

type CXMemberBucket struct {
	Assignee  string      `json:"assignee"`
	Tasks     []CXTaskRow `json:"tasks"`
	Completed []CXTaskRow `json:"completed"`
	Active    []CXTaskRow `json:"active"`
	Blocked   []CXTaskRow `json:"blocked"`
	Backlog   []CXTaskRow `json:"backlog"`
	Other     []CXTaskRow `json:"other"`
}

func computeMemberStats(items []map[string]any, start, end time.Time) map[string]MemberStats {
	out := map[string]MemberStats{}
	closedStatuses := map[string]bool{"closed": true, "done": true, "resolved": true, "rejected": true}

	for _, wp := range items {
		member := linkTitle(wp, "assignee", "Unassigned")
		status := strings.ToLower(linkTitle(wp, "status", ""))
		s := out[member]

		if t, err := parseISO(str(wp["createdAt"])); err == nil && !t.Before(start) && !t.After(end) {
			s.Created++
		}
		if t, err := parseISO(str(wp["updatedAt"])); err == nil && !t.Before(start) && !t.After(end) {
			s.Updated++
			if closedStatuses[status] {
				s.Closed++
			}
		}
		out[member] = s
	}
	return out
}

func sortedKeys(m map[string]MemberStats) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.ToLower(keys[i]) < strings.ToLower(keys[j])
	})
	return keys
}

func sortedStringKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func cxClassifyStatus(status string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	switch {
	case s == "":
		return "other"
	case strings.Contains(s, "backlog"):
		return "backlog"
	case strings.Contains(s, "blocked"):
		return "blocked"
	case strings.Contains(s, "done") || strings.Contains(s, "closed") || strings.Contains(s, "resolved"):
		return "completed"
	case strings.Contains(s, "in progress") || strings.Contains(s, "code review") || strings.Contains(s, "qa: in progress") || strings.Contains(s, "review"):
		return "active"
	default:
		return "other"
	}
}

func cxTaskRow(wp map[string]any) CXTaskRow {
	return CXTaskRow{
		ID:        str(wp["id"]),
		Subject:   str(wp["subject"]),
		Status:    linkTitle(wp, "status", "Unknown"),
		Assignee:  linkTitle(wp, "assignee", "Unassigned"),
		UpdatedAt: str(wp["updatedAt"]),
		CreatedAt: str(wp["createdAt"]),
	}
}

func cmdCXTaskBuckets(c *Client, project string, asJSON bool) {
	pid, err := resolveProject(c, project)
	if err != nil {
		fatalf("%v", err)
	}
	filters := []any{map[string]any{"project": map[string]any{"operator": "=", "values": []string{pid}}}}
	items, err := c.fetchAllPages("/work_packages", map[string]any{"filters": filters})
	if err != nil {
		fatalf("%v", err)
	}

	buckets := map[string]*CXMemberBucket{}
	for _, wp := range items {
		row := cxTaskRow(wp)
		name := row.Assignee
		if _, ok := buckets[name]; !ok {
			buckets[name] = &CXMemberBucket{Assignee: name}
		}
		b := buckets[name]
		b.Tasks = append(b.Tasks, row)
		switch cxClassifyStatus(row.Status) {
		case "completed":
			b.Completed = append(b.Completed, row)
		case "active":
			b.Active = append(b.Active, row)
		case "blocked":
			b.Blocked = append(b.Blocked, row)
		case "backlog":
			b.Backlog = append(b.Backlog, row)
		default:
			b.Other = append(b.Other, row)
		}
	}

	out := make([]CXMemberBucket, 0, len(buckets))
	for _, k := range sortedKeysFromBuckets(buckets) {
		out = append(out, *buckets[k])
	}
	if asJSON {
		printJSON(out)
		return
	}
	for _, b := range out {
		fmt.Printf("%s\tactive=%d\tcompleted=%d\tblocked=%d\tbacklog=%d\tother=%d\n", b.Assignee, len(b.Active), len(b.Completed), len(b.Blocked), len(b.Backlog), len(b.Other))
	}
}

func sortedKeysFromBuckets(m map[string]*CXMemberBucket) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.ToLower(keys[i]) < strings.ToLower(keys[j])
	})
	return keys
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

func requireFlag(fs *flag.FlagSet, name, value string) {
	if strings.TrimSpace(value) == "" {
		fmt.Fprintf(os.Stderr, "missing required flag: --%s\n\n", name)
		fs.Usage()
		os.Exit(2)
	}
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func cmdVersion() {
	fmt.Printf("openproject-ops %s\n", version)
}

func cmdInitConfig(outPath string) {
	if strings.TrimSpace(outPath) == "" {
		outPath = filepath.Join(".secrets", defaultConfigFileName)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fatalf("create config dir: %v", err)
	}
	template := fmt.Sprintf(`{
  "base_url": %q,
  "token": "PASTE_OPENPROJECT_API_TOKEN_HERE",
  "api_path": "/api/v3",
  "parallel": 4,
  "page_size": 200
}
`, defaultConfigExampleURL)
	if err := os.WriteFile(outPath, []byte(template), 0o600); err != nil {
		fatalf("write config file: %v", err)
	}
	fmt.Printf("Config template written to %s\n", outPath)
	fmt.Println("Next step: edit token/base_url, then run: go run . permissions")
}

func normalizeActionHref(actionHref string) string {
	actionHref = strings.TrimSpace(actionHref)
	actionHref = strings.TrimPrefix(actionHref, "/")
	actionHref = strings.TrimPrefix(actionHref, "api/v3/")
	actionHref = strings.TrimPrefix(actionHref, "actions/")
	return actionHref
}

func hasAnyLink(item map[string]any, keys ...string) bool {
	links, _ := item["_links"].(map[string]any)
	for _, k := range keys {
		if _, ok := links[k]; ok {
			return true
		}
	}
	return false
}

func currentUserFromRoot(c *Client) (map[string]any, error) {
	root, err := c.get("/", nil)
	if err != nil {
		return nil, err
	}
	links, _ := root["_links"].(map[string]any)
	userLink, _ := links["user"].(map[string]any)
	href := str(userLink["href"])
	if href == "" {
		return nil, fmt.Errorf("user link not present in /api/v3 root")
	}
	user, err := c.get(toRelativeAPIPath(c, href), nil)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func capabilitiesFor(c *Client, principalID, contextID string) ([]map[string]any, error) {
	filters := []any{
		map[string]any{"principal": map[string]any{"operator": "=", "values": []string{principalID}}},
		map[string]any{"context": map[string]any{"operator": "=", "values": []string{contextID}}},
	}
	return c.fetchAllPages("/capabilities", map[string]any{"filters": filters})
}

func cmdPermissions(c *Client, project string, asJSON bool) {
	user, err := currentUserFromRoot(c)
	if err != nil {
		fatalf("resolve current user: %v", err)
	}
	uid := str(user["id"])
	if uid == "" {
		fatalf("current user id is missing")
	}

	// Probe readable resources first.
	projects, projectsErr := c.fetchAllPages("/projects", map[string]any{"pageSize": 50})
	usersOK := true
	if _, err := c.get("/users", map[string]any{"pageSize": 1}); err != nil {
		usersOK = false
	}
	statusesOK := true
	if _, err := c.get("/statuses", map[string]any{"pageSize": 1}); err != nil {
		statusesOK = false
	}

	var projectObj map[string]any
	projectID := ""
	projectLabel := "global"
	if strings.TrimSpace(project) != "" {
		projectID, err = resolveProject(c, project)
		if err != nil {
			fatalf("resolve project: %v", err)
		}
		projectObj, err = c.get("/projects/"+projectID, nil)
		if err != nil {
			fatalf("load project: %v", err)
		}
		projectLabel = firstNonEmpty(str(projectObj["name"]), projectID)
	} else if projectsErr == nil && len(projects) > 0 {
		projectObj = projects[0]
		projectID = str(projectObj["id"])
		projectLabel = firstNonEmpty(str(projectObj["name"]), projectID)
	}

	// Capabilities (global + optional project context) as additional signal.
	actions := map[string]bool{}
	captureActions := func(items []map[string]any) {
		for _, capItem := range items {
			links, _ := capItem["_links"].(map[string]any)
			action, _ := links["action"].(map[string]any)
			a := normalizeActionHref(str(action["href"]))
			if a != "" {
				actions[a] = true
			}
		}
	}
	if caps, err := capabilitiesFor(c, uid, "g"); err == nil {
		captureActions(caps)
	}
	if projectID != "" {
		if caps, err := capabilitiesFor(c, uid, "p"+projectID); err == nil {
			captureActions(caps)
		}
	}
	actionList := make([]string, 0, len(actions))
	for a := range actions {
		actionList = append(actionList, a)
	}
	sort.Strings(actionList)

	usable := map[string]bool{
		"projects:list":   projectsErr == nil,
		"projects:create": actions["projects/create"],
		"projects:update": actions["projects/update"],
		"projects:delete": actions["projects/delete"],
		"tasks:list":      false,
		"tasks:view":      false,
		"tasks:create":    actions["work_packages/create"],
		"tasks:update":    actions["work_packages/update"],
		"tasks:delete":    actions["work_packages/delete"],
		"users:list":      usersOK || actions["users/read"],
		"statuses:list":   statusesOK || actions["statuses/read"],
		"reports:member":  false,
		"reports:weekly":  false,
	}

	if projectObj != nil {
		if hasAnyLink(projectObj, "createWorkPackage", "createWorkPackageImmediately") {
			usable["tasks:create"] = true
		}
		if hasAnyLink(projectObj, "update", "updateImmediately") {
			usable["projects:update"] = true
		}
		if hasAnyLink(projectObj, "delete") {
			usable["projects:delete"] = true
		}

		filters := []any{map[string]any{"project": map[string]any{"operator": "=", "values": []string{projectID}}}}
		if wp, err := c.get("/work_packages", map[string]any{"filters": filters, "pageSize": 1}); err == nil {
			usable["tasks:list"] = true
			usable["tasks:view"] = true
			usable["reports:member"] = true
			usable["reports:weekly"] = true
			elems := flattenElements(wp)
			if len(elems) > 0 {
				item := elems[0]
				if hasAnyLink(item, "update", "updateImmediately", "edit") {
					usable["tasks:update"] = true
				}
				if hasAnyLink(item, "delete") {
					usable["tasks:delete"] = true
				}
			}
		}
	}

	usable["generic_api_call:get"] = true
	usable["generic_api_call:write"] = usable["projects:create"] || usable["projects:update"] || usable["projects:delete"] || usable["tasks:create"] || usable["tasks:update"] || usable["tasks:delete"]

	if asJSON {
		printJSON(map[string]any{
			"user": map[string]any{
				"id":     uid,
				"name":   str(user["name"]),
				"login":  str(user["login"]),
				"email":  str(user["email"]),
				"admin":  user["admin"],
				"status": str(user["status"]),
			},
			"evaluatedProject":     projectLabel,
			"projectId":            projectID,
			"capabilityActions":    actionList,
			"toolCommandAccess":    usable,
			"requiredUserConfig":   []string{"base_url", "token"},
			"recommendedNextSteps": []string{"fill .secrets/openproject.config.json", "run go run . permissions", "run go run . projects"},
		})
		return
	}

	fmt.Printf("User: %s (%s) | login=%s | admin=%v\n", str(user["name"]), uid, str(user["login"]), user["admin"])
	fmt.Printf("Evaluated project context: %s\n", projectLabel)
	fmt.Printf("Capability actions discovered: %d\n", len(actionList))
	fmt.Println("Likely allowed in this tool:")
	keys := make([]string, 0, len(usable))
	for k := range usable {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if usable[k] {
			fmt.Printf("  ✅ %s\n", k)
		} else {
			fmt.Printf("  ❌ %s\n", k)
		}
	}
	fmt.Println("\nSetup needed for new users:")
	fmt.Printf("  1) Copy openproject.config.example.json to .secrets/%s\n", defaultConfigFileName)
	fmt.Println("  2) Set: base_url + token (api_path/parallel/page_size optional)")
	fmt.Println("  3) Run: go run . permissions")
	fmt.Println("  4) Start with: go run . projects")
}

func cmdProjects(c *Client, asJSON bool) {
	elems, err := c.fetchAllPages("/projects", nil)
	if err != nil {
		fatalf("%v", err)
	}
	if asJSON {
		printJSON(elems)
		return
	}
	for _, p := range elems {
		fmt.Printf("%s\t%s\t%s\n", str(p["id"]), str(p["identifier"]), str(p["name"]))
	}
}

func cmdProjectStatus(c *Client, project string, asJSON bool) {
	pid, err := resolveProject(c, project)
	if err != nil {
		fatalf("%v", err)
	}
	proj, err := c.get("/projects/"+pid, nil)
	if err != nil {
		fatalf("%v", err)
	}

	filters := []any{
		map[string]any{"project": map[string]any{"operator": "=", "values": []string{pid}}},
	}
	items, err := c.fetchAllPages("/work_packages", map[string]any{"filters": filters})
	if err != nil {
		fatalf("%v", err)
	}

	counts := map[string]int{}
	for _, wp := range items {
		counts[linkTitle(wp, "status", "Unknown")]++
	}

	if asJSON {
		printJSON(map[string]any{
			"project":    str(proj["name"]),
			"identifier": str(proj["identifier"]),
			"totalTasks": len(items),
			"byStatus":   counts,
		})
		return
	}
	fmt.Printf("Project: %s (%s)\n", str(proj["name"]), firstNonEmpty(str(proj["identifier"]), pid))
	fmt.Printf("Total tasks: %d\n", len(items))
	fmt.Println("By status:")
	for _, k := range sortedStringKeys(counts) {
		fmt.Printf("  %s: %d\n", k, counts[k])
	}
}

func cmdTasksList(c *Client, project string, limit int, asJSON bool) {
	pid, err := resolveProject(c, project)
	if err != nil {
		fatalf("%v", err)
	}
	filters := []any{map[string]any{"project": map[string]any{"operator": "=", "values": []string{pid}}}}
	data, err := c.get("/work_packages", map[string]any{"filters": filters, "pageSize": limit})
	if err != nil {
		fatalf("%v", err)
	}
	elems := flattenElements(data)
	if asJSON {
		printJSON(elems)
		return
	}
	for _, wp := range elems {
		fmt.Printf("#%s [%s] %s | %s | %s\n",
			str(wp["id"]),
			linkTitle(wp, "status", "?"),
			str(wp["subject"]),
			linkTitle(wp, "assignee", "Unassigned"),
			str(wp["updatedAt"]),
		)
	}
}

func cmdTaskView(c *Client, id string, asJSON bool) {
	data, err := c.get("/work_packages/"+id, nil)
	if err != nil {
		fatalf("%v", err)
	}
	if asJSON {
		printJSON(data)
		return
	}
	desc := ""
	if d, ok := data["description"].(map[string]any); ok {
		desc = str(d["raw"])
	}
	fmt.Printf("ID:          #%s\n", str(data["id"]))
	fmt.Printf("Subject:     %s\n", str(data["subject"]))
	fmt.Printf("Type:        %s\n", linkTitle(data, "type", "?"))
	fmt.Printf("Status:      %s\n", linkTitle(data, "status", "?"))
	fmt.Printf("Priority:    %s\n", linkTitle(data, "priority", "?"))
	fmt.Printf("Assignee:    %s\n", linkTitle(data, "assignee", "Unassigned"))
	fmt.Printf("Author:      %s\n", linkTitle(data, "author", "?"))
	fmt.Printf("Project:     %s\n", linkTitle(data, "project", "?"))
	fmt.Printf("Created:     %s\n", str(data["createdAt"]))
	fmt.Printf("Updated:     %s\n", str(data["updatedAt"]))
	fmt.Printf("Start date:  %s\n", str(data["startDate"]))
	fmt.Printf("Due date:    %s\n", str(data["dueDate"]))
	fmt.Printf("Done %%:      %s\n", str(data["percentageDone"]))
	if desc != "" {
		fmt.Printf("\nDescription:\n%s\n", desc)
	}
}

func cmdTaskCreate(c *Client, project, title, description, assignee, parent string, asJSON bool) {
	pid, err := resolveProject(c, project)
	if err != nil {
		fatalf("%v", err)
	}
	typeHref, err := resolveTaskTypeHref(c, pid)
	if err != nil {
		fatalf("%v", err)
	}

	links := map[string]any{
		"project": map[string]any{"href": c.apiPath + "/projects/" + pid},
		"type":    map[string]any{"href": typeHref},
	}
	if assignee != "" {
		href, err := resolveUserHref(c, assignee)
		if err != nil {
			fatalf("resolve assignee: %v", err)
		}
		if href == "" {
			fmt.Fprintf(os.Stderr, "warning: assignee %q not found; creating unassigned\n", assignee)
		} else {
			links["assignee"] = map[string]any{"href": href}
		}
	}
	if parent != "" {
		links["parent"] = map[string]any{"href": c.apiPath + "/work_packages/" + strings.TrimPrefix(parent, "#")}
	}

	body := map[string]any{
		"subject": title,
		"description": map[string]any{
			"format": "markdown",
			"raw":    description,
		},
		"_links": links,
	}

	created, err := c.post("/work_packages", body)
	if err != nil {
		fatalf("%v", err)
	}
	if asJSON {
		printJSON(created)
		return
	}
	fmt.Printf("Created task #%s: %s\n", str(created["id"]), str(created["subject"]))
}

func cmdTaskUpdate(c *Client, id string, fields map[string]any, asJSON bool) {
	updated, err := c.patch("/work_packages/"+id, fields)
	if err != nil {
		fatalf("%v", err)
	}
	if asJSON {
		printJSON(updated)
		return
	}
	fmt.Printf("Updated task #%s: %s\n", str(updated["id"]), str(updated["subject"]))
}

func cmdTaskDelete(c *Client, id string) {
	_, err := c.delete("/work_packages/" + id)
	if err != nil {
		fatalf("%v", err)
	}
	fmt.Printf("Deleted task #%s\n", id)
}

func cmdUsers(c *Client, asJSON bool) {
	items, err := c.fetchAllPages("/users", nil)
	if err != nil {
		fatalf("%v", err)
	}
	if asJSON {
		printJSON(items)
		return
	}
	for _, u := range items {
		fmt.Printf("%s\t%s\t%s\t%s\n",
			str(u["id"]),
			str(u["login"]),
			str(u["name"]),
			str(u["email"]),
		)
	}
}

func cmdStatuses(c *Client, asJSON bool) {
	data, err := c.get("/statuses", map[string]any{"pageSize": c.pageSize})
	if err != nil {
		fatalf("%v", err)
	}
	elems := flattenElements(data)
	if asJSON {
		printJSON(elems)
		return
	}
	for _, s := range elems {
		closed := ""
		if b, ok := s["isClosed"].(bool); ok && b {
			closed = " [closed]"
		}
		fmt.Printf("%s\t%s%s\n", str(s["id"]), str(s["name"]), closed)
	}
}

func cmdMemberActivity(c *Client, project, from, to string, asJSON bool) {
	pid, err := resolveProject(c, project)
	if err != nil {
		fatalf("%v", err)
	}
	start, end, err := parseDateRange(from, to)
	if err != nil {
		fatalf("%v", err)
	}

	filters := []any{
		map[string]any{"project": map[string]any{"operator": "=", "values": []string{pid}}},
		map[string]any{"updatedAt": map[string]any{
			"operator": "<>d",
			"values":   []string{start.Format(time.RFC3339), end.Format(time.RFC3339)},
		}},
	}
	items, err := c.fetchAllPages("/work_packages", map[string]any{"filters": filters})
	if err != nil {
		fatalf("%v", err)
	}

	stats := computeMemberStats(items, start, end)
	if asJSON {
		printJSON(stats)
		return
	}
	fmt.Println("member\tcreated\tupdated\tclosed")
	for _, m := range sortedKeys(stats) {
		s := stats[m]
		fmt.Printf("%s\t%d\t%d\t%d\n", m, s.Created, s.Updated, s.Closed)
	}
}

func cmdWeeklyReport(c *Client, project, weekStartStr, outPath string) {
	pid, err := resolveProject(c, project)
	if err != nil {
		fatalf("%v", err)
	}
	proj, err := c.get("/projects/"+pid, nil)
	if err != nil {
		fatalf("%v", err)
	}

	var start time.Time
	if weekStartStr != "" {
		start, err = time.Parse("2006-01-02", weekStartStr)
		if err != nil {
			fatalf("invalid --week-start: %v", err)
		}
		start = startOfDay(start)
	} else {
		start = mondayOfWeek()
	}
	end := endOfDay(start.AddDate(0, 0, 6))

	filters := []any{
		map[string]any{"project": map[string]any{"operator": "=", "values": []string{pid}}},
		map[string]any{"updatedAt": map[string]any{
			"operator": "<>d",
			"values":   []string{start.Format(time.RFC3339), end.Format(time.RFC3339)},
		}},
	}
	items, err := c.fetchAllPages("/work_packages", map[string]any{"filters": filters})
	if err != nil {
		fatalf("%v", err)
	}

	counts := map[string]int{}
	for _, wp := range items {
		counts[linkTitle(wp, "status", "Unknown")]++
	}
	stats := computeMemberStats(items, start, end)

	report := buildWeeklyReport(
		firstNonEmpty(str(proj["name"]), project),
		start, end, counts, stats, items,
	)

	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(report), 0o644); err != nil {
			fatalf("write report: %v", err)
		}
		fmt.Printf("Report written to %s\n", outPath)
	} else {
		fmt.Print(report)
	}
}

func buildWeeklyReport(label string, start, end time.Time, counts map[string]int, stats map[string]MemberStats, items []map[string]any) string {
	var b strings.Builder

	b.WriteString("# Weekly Report — " + label + "\n\n")
	b.WriteString(fmt.Sprintf("Period: **%s** to **%s**\n\n", start.Format("2006-01-02"), end.Format("2006-01-02")))

	// Status breakdown.
	b.WriteString("## Status breakdown\n\n")
	if len(counts) == 0 {
		b.WriteString("- No work packages found\n\n")
	} else {
		for _, k := range sortedStringKeys(counts) {
			b.WriteString(fmt.Sprintf("- %s: %d\n", k, counts[k]))
		}
		b.WriteString("\n")
	}

	// Member activity table.
	b.WriteString("## Member activity\n\n")
	b.WriteString("| Member | Created | Updated | Closed |\n")
	b.WriteString("|--------|--------:|--------:|-------:|\n")
	if len(stats) == 0 {
		b.WriteString("| (none) | 0 | 0 | 0 |\n")
	} else {
		for _, k := range sortedKeys(stats) {
			s := stats[k]
			b.WriteString(fmt.Sprintf("| %s | %d | %d | %d |\n", k, s.Created, s.Updated, s.Closed))
		}
	}
	b.WriteString("\n")

	// Notable tasks.
	b.WriteString("## Notable updated tasks\n\n")
	sort.Slice(items, func(i, j int) bool {
		return str(items[i]["updatedAt"]) > str(items[j]["updatedAt"])
	})
	if len(items) == 0 {
		b.WriteString("- No updates in this period\n")
	} else {
		limit := maxNotableTasks
		if len(items) < limit {
			limit = len(items)
		}
		for _, wp := range items[:limit] {
			b.WriteString(fmt.Sprintf("- #%s [%s] %s — %s (updated %s)\n",
				str(wp["id"]),
				linkTitle(wp, "status", "?"),
				str(wp["subject"]),
				linkTitle(wp, "assignee", "Unassigned"),
				str(wp["updatedAt"]),
			))
		}
	}
	b.WriteString("\n")
	return b.String()
}

func cmdEndpoints(c *Client, search string) {
	spec, err := c.openAPISpec()
	if err != nil {
		fatalf("%v", err)
	}
	paths, _ := spec["paths"].(map[string]any)
	if len(paths) == 0 {
		fatalf("no paths found in OpenAPI spec")
	}

	needle := strings.ToLower(strings.TrimSpace(search))
	var rows []string
	for p, v := range paths {
		methods, _ := v.(map[string]any)
		for method, opVal := range methods {
			mu := strings.ToUpper(method)
			switch mu {
			case "GET", "POST", "PATCH", "PUT", "DELETE", "HEAD", "OPTIONS":
			default:
				continue
			}
			op, _ := opVal.(map[string]any)
			opID := str(op["operationId"])
			line := fmt.Sprintf("%s\t%s\t%s", mu, p, opID)
			if needle == "" || strings.Contains(strings.ToLower(line), needle) {
				rows = append(rows, line)
			}
		}
	}
	sort.Strings(rows)
	for _, r := range rows {
		fmt.Println(r)
	}
}

func cmdAPICall(c *Client, method, path, queryJSON, bodyJSON string) {
	q, err := parseJSONFlag(queryJSON)
	if err != nil {
		fatalf("invalid --query-json: %v", err)
	}
	var body any
	if bodyJSON != "" {
		body, err = parseJSONFlag(bodyJSON)
		if err != nil {
			fatalf("invalid --body-json: %v", err)
		}
	}
	resp, err := c.request(context.Background(), strings.ToUpper(method), path, q, body)
	if err != nil {
		fatalf("%v", err)
	}
	printJSON(resp)
}

func parseJSONFlag(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Usage / main
// ---------------------------------------------------------------------------

func usage() {
	fmt.Print(`openproject-ops — OpenProject API v3 CLI

Usage:
  openproject-ops <command> [flags]

Commands:
  projects                                    List all projects
  project-status  --project <id|identifier>   Project summary with task counts
  tasks-list      --project <id>  [--limit N] List work packages
  cx-task-buckets --project <id>              Group full project tasks by assignee/status buckets
  task-view       --id <wp-id>                View a single work package
  task-create     --project <id> --title <t>  Create a work package
                  [--description <d>] [--assignee <name|id>]
  task-update     --id <wp-id> --fields-json '{...}'
                                              Patch a work package
  task-delete     --id <wp-id>                Delete a work package
  users                                       List all users
  statuses                                    List all statuses
  member-activity --project <id>              Per-member stats in date range
                  --from YYYY-MM-DD --to YYYY-MM-DD
  weekly-report   --project <id>              Markdown weekly report
                  [--week-start YYYY-MM-DD] [--out file.md]
  endpoints       [--search <text>]           List OpenAPI endpoints
  api-call        --method GET --path /...    Call any API v3 endpoint
                  [--query-json '{...}'] [--body-json '{...}']
  permissions     [--project <id>]            Inspect token-based capabilities
                  [--json]
  init-config     [--out .secrets/openproject.config.json]
                                              Create editable user config template
  version                                     Print version

Global flags (supported by most commands):
  --json          Machine-readable JSON output

Environment:
  OPENPROJECT_BASE_URL    (required) e.g. https://your-openproject.example.com
  OPENPROJECT_TOKEN       (required) API token
  OPENPROJECT_API_PATH    (optional) default: /api/v3
  OPENPROJECT_CONFIG_FILE (optional) path to JSON config file
  OPENPROJECT_ENV_FILE    (optional) path to .env credential file
  OPENPROJECT_PARALLEL    (optional) parallel page fetch workers, default 4 (max 16)
  OPENPROJECT_PAGE_SIZE   (optional) collection page size, default 200 (max 1000)

Auto-load order:
  1) .secrets/openproject.config.json
  2) .secrets/openproject.env
  3) shell env vars (highest priority)
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	loadOpenProjectConfig()

	cmd := os.Args[1]
	if cmd == "version" || cmd == "--version" || cmd == "-v" {
		cmdVersion()
		return
	}
	if cmd == "help" || cmd == "--help" || cmd == "-h" {
		usage()
		return
	}
	if cmd == "init-config" {
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		out := fs.String("out", filepath.Join(".secrets", defaultConfigFileName), "")
		_ = fs.Parse(os.Args[2:])
		cmdInitConfig(*out)
		return
	}

	client := NewClient(
		envOrDie("OPENPROJECT_BASE_URL", ""),
		envOrDie("OPENPROJECT_TOKEN", ""),
		envOrDie("OPENPROJECT_API_PATH", defaultAPIPath),
		envInt("OPENPROJECT_PARALLEL", defaultParallel, 1, maxParallel),
		envInt("OPENPROJECT_PAGE_SIZE", defaultPageSize, 1, maxPageSize),
	)

	switch cmd {
	case "projects":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		asJSON := fs.Bool("json", false, "")
		_ = fs.Parse(os.Args[2:])
		cmdProjects(client, *asJSON)

	case "project-status":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		project := fs.String("project", "", "Project identifier or numeric ID")
		asJSON := fs.Bool("json", false, "")
		_ = fs.Parse(os.Args[2:])
		requireFlag(fs, "project", *project)
		cmdProjectStatus(client, *project, *asJSON)

	case "tasks-list":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		project := fs.String("project", "", "")
		limit := fs.Int("limit", defaultTasksLimit, "")
		asJSON := fs.Bool("json", false, "")
		_ = fs.Parse(os.Args[2:])
		requireFlag(fs, "project", *project)
		cmdTasksList(client, *project, *limit, *asJSON)

	case "cx-task-buckets":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		project := fs.String("project", "", "")
		asJSON := fs.Bool("json", false, "")
		_ = fs.Parse(os.Args[2:])
		requireFlag(fs, "project", *project)
		cmdCXTaskBuckets(client, *project, *asJSON)

	case "task-view":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		id := fs.String("id", "", "Work package ID")
		asJSON := fs.Bool("json", false, "")
		_ = fs.Parse(os.Args[2:])
		requireFlag(fs, "id", *id)
		cmdTaskView(client, *id, *asJSON)

	case "task-create":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		project := fs.String("project", "", "")
		title := fs.String("title", "", "")
		desc := fs.String("description", "", "")
		assignee := fs.String("assignee", "", "")
		parent := fs.String("parent", "", "Parent work package ID")
		asJSON := fs.Bool("json", false, "")
		_ = fs.Parse(os.Args[2:])
		requireFlag(fs, "project", *project)
		requireFlag(fs, "title", *title)
		cmdTaskCreate(client, *project, *title, *desc, *assignee, *parent, *asJSON)

	case "task-update":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		id := fs.String("id", "", "Work package ID")
		fieldsJSON := fs.String("fields-json", "", "JSON patch body")
		asJSON := fs.Bool("json", false, "")
		_ = fs.Parse(os.Args[2:])
		requireFlag(fs, "id", *id)
		requireFlag(fs, "fields-json", *fieldsJSON)
		fields, err := parseJSONFlag(*fieldsJSON)
		if err != nil {
			fatalf("invalid --fields-json: %v", err)
		}
		cmdTaskUpdate(client, *id, fields, *asJSON)

	case "task-delete":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		id := fs.String("id", "", "Work package ID")
		_ = fs.Parse(os.Args[2:])
		requireFlag(fs, "id", *id)
		cmdTaskDelete(client, *id)

	case "users":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		asJSON := fs.Bool("json", false, "")
		_ = fs.Parse(os.Args[2:])
		cmdUsers(client, *asJSON)

	case "statuses":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		asJSON := fs.Bool("json", false, "")
		_ = fs.Parse(os.Args[2:])
		cmdStatuses(client, *asJSON)

	case "member-activity":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		project := fs.String("project", "", "")
		from := fs.String("from", "", "")
		to := fs.String("to", "", "")
		asJSON := fs.Bool("json", false, "")
		_ = fs.Parse(os.Args[2:])
		requireFlag(fs, "project", *project)
		requireFlag(fs, "from", *from)
		requireFlag(fs, "to", *to)
		cmdMemberActivity(client, *project, *from, *to, *asJSON)

	case "weekly-report":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		project := fs.String("project", "", "")
		weekStart := fs.String("week-start", "", "")
		out := fs.String("out", "", "")
		_ = fs.Parse(os.Args[2:])
		requireFlag(fs, "project", *project)
		cmdWeeklyReport(client, *project, *weekStart, *out)

	case "endpoints":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		search := fs.String("search", "", "")
		_ = fs.Parse(os.Args[2:])
		cmdEndpoints(client, *search)

	case "api-call":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		method := fs.String("method", "GET", "")
		path := fs.String("path", "", "")
		queryJSON := fs.String("query-json", "", "")
		bodyJSON := fs.String("body-json", "", "")
		_ = fs.Parse(os.Args[2:])
		requireFlag(fs, "path", *path)
		cmdAPICall(client, *method, *path, *queryJSON, *bodyJSON)

	case "permissions":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		project := fs.String("project", "", "Project id/identifier for project-scoped check")
		asJSON := fs.Bool("json", false, "")
		_ = fs.Parse(os.Args[2:])
		cmdPermissions(client, *project, *asJSON)

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		os.Exit(2)
	}
}
