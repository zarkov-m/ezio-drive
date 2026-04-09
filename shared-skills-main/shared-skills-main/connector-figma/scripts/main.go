package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func newClient() (*Client, error) {
	tok := strings.TrimSpace(os.Getenv("FIGMA_TOKEN"))
	if tok == "" {
		return nil, fmt.Errorf("FIGMA_TOKEN is required")
	}
	base := strings.TrimSpace(os.Getenv("FIGMA_API_BASE"))
	if base == "" {
		base = "https://api.figma.com/v1"
	}
	return &Client{baseURL: strings.TrimRight(base, "/"), token: tok, http: &http.Client{}}, nil
}

func (c *Client) request(method, path string, query map[string]string, body any) ([]byte, error) {
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	for k, v := range query {
		if strings.TrimSpace(v) != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()

	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewBuffer(b)
	}

	req, err := http.NewRequest(method, u.String(), rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Figma-Token", c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("figma api %s %s failed: status=%d body=%s", method, path, resp.StatusCode, string(out))
	}
	return out, nil
}

func printJSON(raw []byte) error {
	var anyJSON any
	if err := json.Unmarshal(raw, &anyJSON); err != nil {
		fmt.Println(string(raw))
		return nil
	}
	pretty, err := json.MarshalIndent(anyJSON, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(pretty))
	return nil
}

func cmdFile(c *Client, args []string) error {
	fs := flag.NewFlagSet("file", flag.ExitOnError)
	key := fs.String("key", "", "Figma file key")
	depth := fs.String("depth", "", "Optional depth")
	branch := fs.String("branch", "", "Optional branch_data=true|false")
	_ = fs.Parse(args)
	if *key == "" {
		return fmt.Errorf("--key is required")
	}
	q := map[string]string{"depth": *depth, "branch_data": *branch}
	b, err := c.request(http.MethodGet, "/files/"+*key, q, nil)
	if err != nil {
		return err
	}
	return printJSON(b)
}

func cmdNodes(c *Client, args []string) error {
	fs := flag.NewFlagSet("nodes", flag.ExitOnError)
	key := fs.String("key", "", "Figma file key")
	ids := fs.String("ids", "", "Comma-separated node ids (e.g. 1:2,3:4)")
	_ = fs.Parse(args)
	if *key == "" || *ids == "" {
		return fmt.Errorf("--key and --ids are required")
	}
	b, err := c.request(http.MethodGet, "/files/"+*key+"/nodes", map[string]string{"ids": *ids}, nil)
	if err != nil {
		return err
	}
	return printJSON(b)
}

func cmdImages(c *Client, args []string) error {
	fs := flag.NewFlagSet("images", flag.ExitOnError)
	key := fs.String("key", "", "Figma file key")
	ids := fs.String("ids", "", "Comma-separated node ids")
	format := fs.String("format", "png", "png|jpg|svg|pdf")
	scale := fs.String("scale", "1", "Scale factor")
	_ = fs.Parse(args)
	if *key == "" || *ids == "" {
		return fmt.Errorf("--key and --ids are required")
	}
	q := map[string]string{"ids": *ids, "format": *format, "scale": *scale}
	b, err := c.request(http.MethodGet, "/images/"+*key, q, nil)
	if err != nil {
		return err
	}
	return printJSON(b)
}

func cmdCommentsList(c *Client, args []string) error {
	fs := flag.NewFlagSet("comments-list", flag.ExitOnError)
	key := fs.String("key", "", "Figma file key")
	_ = fs.Parse(args)
	if *key == "" {
		return fmt.Errorf("--key is required")
	}
	b, err := c.request(http.MethodGet, "/files/"+*key+"/comments", nil, nil)
	if err != nil {
		return err
	}
	return printJSON(b)
}

func cmdCommentsAdd(c *Client, args []string) error {
	fs := flag.NewFlagSet("comments-add", flag.ExitOnError)
	key := fs.String("key", "", "Figma file key")
	message := fs.String("message", "", "Comment text")
	x := fs.Float64("x", 0, "X coordinate")
	y := fs.Float64("y", 0, "Y coordinate")
	_ = fs.Parse(args)
	if *key == "" || strings.TrimSpace(*message) == "" {
		return fmt.Errorf("--key and --message are required")
	}
	body := map[string]any{
		"message": *message,
		"client_meta": map[string]float64{
			"x": *x,
			"y": *y,
		},
	}
	b, err := c.request(http.MethodPost, "/files/"+*key+"/comments", nil, body)
	if err != nil {
		return err
	}
	return printJSON(b)
}

func usage() {
	fmt.Println("connector-figma (Go)")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  file           --key <FILE_KEY> [--depth N] [--branch true|false]")
	fmt.Println("  nodes          --key <FILE_KEY> --ids <ID1,ID2>")
	fmt.Println("  images         --key <FILE_KEY> --ids <ID1,ID2> [--format png|jpg|svg|pdf] [--scale 2]")
	fmt.Println("  comments-list  --key <FILE_KEY>")
	fmt.Println("  comments-add   --key <FILE_KEY> --message <TEXT> [--x 0 --y 0]")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	c, err := newClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "file":
		err = cmdFile(c, args)
	case "nodes":
		err = cmdNodes(c, args)
	case "images":
		err = cmdImages(c, args)
	case "comments-list":
		err = cmdCommentsList(c, args)
	case "comments-add":
		err = cmdCommentsAdd(c, args)
	default:
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
