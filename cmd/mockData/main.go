// (moved functions will be defined below)
package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	cryptHash "github.com/i5heu/ouroboros-crypt/pkg/hash"
	keys "github.com/i5heu/ouroboros-crypt/pkg/keys"
	ouroboros "github.com/i5heu/ouroboros-db"
)

// rateLimiter controls requests per second for external API calls.
type rateLimiter struct {
	tokens chan struct{}
}

func newRateLimiter(rps int) *rateLimiter {
	if rps <= 0 {
		rps = 1
	}
	rl := &rateLimiter{tokens: make(chan struct{}, rps)}
	// Seed the bucket to allow immediate bursts up to capacity
	for i := 0; i < rps; i++ {
		rl.tokens <- struct{}{}
	}
	interval := time.Second / time.Duration(rps)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			select {
			case rl.tokens <- struct{}{}:
			default:
				// channel full, skip refill
			}
		}
	}()
	return rl
}

func (rl *rateLimiter) acquire() {
	<-rl.tokens
}

// global Wikipedia API rate limiter (default 100 rps). Create in main.
var wikiLimiter *rateLimiter

const wikiUserAgent = "ouroboros-mockData/1.0 (+https://github.com/i5heu/ouroboros-db)"

func wikiGET(u string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", wikiUserAgent)
	return http.DefaultClient.Do(req)
}

// Wiki section representation
type WikiSection struct {
	Level      int
	Title      string
	Paragraphs []string
}

// fetchRandomWikipediaTitles queries MediaWiki API for random article titles in the given language
func fetchRandomWikipediaTitles(lang string, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	titles := make([]string, 0, limit)
	for len(titles) < limit {
		title, err := fetchRandomWikipediaTitle(lang)
		if err != nil {
			return nil, err
		}
		titles = append(titles, title)
		fmt.Println("fetched wiki title:", title)
	}
	return titles, nil
}

func fetchRandomWikipediaTitle(lang string) (string, error) {
	u := fmt.Sprintf("https://%s.wikipedia.org/api/rest_v1/page/random/title", lang)
	wikiLimiter.acquire()
	resp, err := wikiGET(u)
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		if fallback := extractBadIntegerValue(body); fallback != "" {
			return fetchSpecificWikipediaTitle(lang, fallback)
		}
		return "", fmt.Errorf("wiki random title request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return parseTitleFromBody(body)
}

func fetchSpecificWikipediaTitle(lang, title string) (string, error) {
	encoded := url.PathEscape(title)
	u := fmt.Sprintf("https://%s.wikipedia.org/api/rest_v1/page/title/%s", lang, encoded)
	wikiLimiter.acquire()
	resp, err := wikiGET(u)
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("wiki title request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return parseTitleFromBody(body)
}

func parseTitleFromBody(body []byte) (string, error) {
	var parsed struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Items) > 0 && parsed.Items[0].Title != "" {
		return parsed.Items[0].Title, nil
	}
	if parsed.Title != "" {
		return parsed.Title, nil
	}
	return "", fmt.Errorf("title not found in response")
}

func extractBadIntegerValue(body []byte) string {
	var apiErr struct {
		ErrorKey string `json:"errorKey"`
		Name     string `json:"name"`
		Value    string `json:"value"`
	}
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return ""
	}
	if apiErr.ErrorKey == "badinteger" && apiErr.Name == "id" && apiErr.Value != "" {
		return apiErr.Value
	}
	return ""
}

// fetchArticleMobileSections fetches article sections via the mobile-html endpoint only.
func fetchArticleMobileSections(lang, title string) ([]WikiSection, error) {
	return fetchArticleMobileHTMLSections(lang, title)
}

func fetchArticleMobileHTMLSections(lang, title string) ([]WikiSection, error) {
	encoded := url.PathEscape(title)
	u := fmt.Sprintf("https://%s.wikipedia.org/api/rest_v1/page/mobile-html/%s", lang, encoded)
	wikiLimiter.acquire()
	resp, err := wikiGET(u)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("wiki mobile html request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	sections := collectSectionsFromHTML(doc)
	if len(sections) == 0 {
		return nil, fmt.Errorf("no sections parsed from mobile html")
	}
	return sections, nil
}

func collectSectionsFromHTML(root *html.Node) []WikiSection {
	sections := []WikiSection{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if isSectionNode(n) {
			if ws, ok := buildWikiSectionFromNode(n); ok {
				sections = append(sections, ws)
			}
			// still walk children to capture nested sections
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return sections
}

func isSectionNode(n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode || n.Data != "section" {
		return false
	}
	for _, attr := range n.Attr {
		if attr.Key == "data-mw-section-id" {
			return true
		}
	}
	return false
}

func buildWikiSectionFromNode(n *html.Node) (WikiSection, bool) {
	sectionID := getAttr(n, "data-mw-section-id")
	if sectionID == "" {
		return WikiSection{}, false
	}
	level := 0
	if sectionID != "0" {
		level = headingLevelForSection(n)
	}
	title := headingTitleForSection(n)
	paras := extractParagraphsFromSectionNode(n)
	return WikiSection{Level: level, Title: title, Paragraphs: paras}, true
}

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func headingLevelForSection(n *html.Node) int {
	head := findHeadingNode(n)
	if head == nil {
		return 1
	}
	switch head.Data {
	case "h2":
		return 1
	case "h3":
		return 2
	case "h4":
		return 3
	case "h5":
		return 4
	case "h6":
		return 5
	default:
		return 1
	}
}

func headingTitleForSection(n *html.Node) string {
	head := findHeadingNode(n)
	if head == nil {
		return ""
	}
	return strings.TrimSpace(nodeText(head))
}

func findHeadingNode(n *html.Node) *html.Node {
	var heading *html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if heading != nil {
			return
		}
		if node != n && isSectionNode(node) {
			return
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "h2", "h3", "h4", "h5", "h6":
				heading = node
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
			if heading != nil {
				return
			}
		}
	}
	walk(n)
	return heading
}

func extractParagraphsFromSectionNode(n *html.Node) []string {
	paras := []string{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node != n && isSectionNode(node) {
			return
		}
		if node.Type == html.ElementNode && node.Data == "p" {
			text := strings.TrimSpace(nodeText(node))
			if text != "" {
				paras = append(paras, text)
			}
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return paras
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// extractParagraphsFromHTML extracts visible paragraph strings from a chunk of HTML
func extractParagraphsFromHTML(htmlStr string) []string {
	if htmlStr == "" {
		return nil
	}
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil
	}
	paras := []string{}
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "p" {
			// Collect text content
			var b strings.Builder
			var g func(*html.Node)
			g = func(c *html.Node) {
				if c.Type == html.TextNode {
					b.WriteString(c.Data)
				}
				for c := c.FirstChild; c != nil; c = c.NextSibling {
					g(c)
				}
			}
			g(n)
			s := strings.TrimSpace(b.String())
			if s != "" {
				paras = append(paras, s)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	return paras
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("mockData failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("mockData", flag.ContinueOnError)
	threads := fs.Int("threads", 100, "number of threads to create")
	messages := fs.Int("messages", 30, "messages per thread to create")
	path := fs.String("path", "./data", "data directory (where ouroboros.key will live)")
	httpMode := fs.Bool("http", false, "POST data via API server instead of writing DB directly")
	serverAddr := fs.String("server", "http://localhost:8083", "server address when using HTTP mode")
	otkKeyB64 := fs.String("otk-key", "", "base64 one-time key generated by server (only for HTTP mode)")
	otkNonceB64 := fs.String("otk-nonce", "", "base64 one-time key nonce generated by server (only for HTTP mode)")
	randSeed := fs.Int64("seed", time.Now().UnixNano(), "rand seed - useful for reproducible trees")
	wikiLang := fs.String("wiki-lang", "en", "Wikipedia language (e.g., en, de, fr)")
	wikiRps := fs.Int("wiki-rps", 20, "Max Wikipedia requests per second (<=20)")
	articleFile := fs.String("articles-file", "", "optional file containing article titles (one per line)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// setup logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	slog.SetDefault(logger)

	// create key if missing
	keyFile := filepath.Join(*path, "ouroboros.key")
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		slog.Info("no keyfile found - generating new one", "path", keyFile)
		a, err := keys.NewAsyncCrypt()
		if err != nil {
			return fmt.Errorf("failed to generate keypair: %w", err)
		}
		if err := a.SaveToFile(keyFile); err != nil {
			return fmt.Errorf("failed to save key file: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to stat keyfile: %w", err)
	}

	// init DB - we construct DB only when using direct DB mode (default)
	var db *ouroboros.OuroborosDB
	ctx := context.Background()
	if !*httpMode {
		cfg := ouroboros.Config{Paths: []string{*path}, MinimumFreeGB: 1}
		var err error
		db, err = ouroboros.New(cfg)
		if err != nil {
			return fmt.Errorf("failed to construct DB: %w", err)
		}

		if err := db.Start(ctx); err != nil {
			return fmt.Errorf("failed to start DB: %w", err)
		}
	}

	// seeded rand
	rand.Seed(*randSeed)

	// If httpMode is true, we must obtain browser auth token/kv hash before writing
	var httpAuthData *httpAuth
	if *httpMode {
		if *otkKeyB64 == "" || *otkNonceB64 == "" {
			return fmt.Errorf("http mode requires -otk-key and -otk-nonce (base64)")
		}
		var err error
		httpAuthData, err = initHTTPAuth(*serverAddr, *otkKeyB64, *otkNonceB64)
		if err != nil {
			return fmt.Errorf("failed to initialize HTTP auth: %w", err)
		}
	}

	// create threads concurrently
	var wg sync.WaitGroup
	startTime := time.Now()

	// initialize wiki limiter (default up to 100 rps) - clamp to 100
	rps := *wikiRps
	if rps <= 0 {
		rps = 1
	}
	if rps > 100 {
		rps = 100
	}
	wikiLimiter = newRateLimiter(rps)
	wg.Add(*threads)
	// Prepare list of article titles
	titles := make([]string, 0, *threads)
	if *articleFile != "" {
		data, err := os.ReadFile(*articleFile)
		if err != nil {
			return fmt.Errorf("failed to read articles-file: %w", err)
		}
		for _, line := range bytes.Split(data, []byte("\n")) {
			t := strings.TrimSpace(string(line))
			if t != "" {
				titles = append(titles, t)
			}
			if len(titles) >= *threads {
				break
			}
		}
	}
	if len(titles) < *threads {
		remaining := *threads - len(titles)
		randTitles, err := fetchRandomWikipediaTitles(*wikiLang, remaining)
		if err != nil {
			return fmt.Errorf("failed to fetch random wiki titles: %w", err)
		}
		titles = append(titles, randTitles...)
	}

	for i := 0; i < *threads; i++ {
		go func(tIdx int) {
			defer wg.Done()
			articleTitle := titles[tIdx]
			if *httpMode {
				createThreadHTTP(*serverAddr, httpAuthData, tIdx, *messages, articleTitle, *wikiLang)
			} else {
				createThread(ctx, db, tIdx, *messages, articleTitle, *wikiLang)
			}
		}(i)
	}
	wg.Wait()

	duration := time.Since(startTime)
	fmt.Printf("Seeding completed: %d threads x %d messages in %s\n", *threads, *messages, duration.String())

	if db != nil {
		if err := db.CloseWithoutContext(); err != nil {
			slog.Warn("error closing DB", "error", err)
		}
	}
	return nil
}

// createThread builds a root node followed by a forest of messages that are
// parented to previous nodes in the same thread to build nested levels.
func createThread(ctx context.Context, db *ouroboros.OuroborosDB, threadIdx, messages int, articleTitle, lang string) {
	// Each thread will be a Wikipedia article root
	// If articleTitle empty, fallback to random generated thread
	rootContent := []byte(fmt.Sprintf("Thread %d root", threadIdx))
	if articleTitle != "" {
		// fetch article and create nodes by section
		if err := createThreadFromArticle(ctx, db, lang, articleTitle, messages); err != nil {
			slog.Error("failed to create thread from article", "thread", threadIdx, "error", err)
		}
		return
	}
	rootHash, err := db.StoreData(ctx, rootContent, ouroboros.StoreOptions{MimeType: "text/plain; charset=utf-8"})
	if err != nil {
		slog.Error("failed to create thread root", "thread", threadIdx, "error", err)
		return
	}

	// keep a slice of created keys within this thread so new messages can link to them
	var keys []cryptHash.Hash
	keys = append(keys, rootHash)

	// Decide a maximum extra depth for this thread randomly to introduce variance
	maxDepth := rand.Intn(8) + 1 // 1..8

	// Create messages
	for m := 1; m < messages; m++ {
		// choose a parent; sometimes choose the latest to create deep chains, sometimes random earlier for branching
		choice := rand.Intn(100)
		var parent cryptHash.Hash
		if choice < 60 { // bias towards recent nodes
			parent = keys[len(keys)-1]
		} else if choice < 90 { // randomly select within recent half
			idx := len(keys) - 1 - rand.Intn(min(len(keys), maxDepth))
			parent = keys[idx]
		} else { // pick a random node from earlier
			parent = keys[rand.Intn(len(keys))]
		}

		// create content; sometimes make it binary
		var content []byte
		var mime string
		if rand.Intn(100) < 10 { // 10% binary
			content = []byte{0xde, 0xad, byte(threadIdx & 0xff), byte(m & 0xff)}
			mime = "application/octet-stream"
		} else {
			depth := computeDepth(keys, parent)
			content = []byte(fmt.Sprintf("Thread %d message %d depth %d parent %s", threadIdx, m, depth, parent.String()))
			mime = "text/plain; charset=utf-8"
		}

		opts := ouroboros.StoreOptions{Parent: parent, MimeType: mime}
		hash, err := db.StoreData(ctx, content, opts)
		if err != nil {
			slog.Warn("failed to store message", "thread", threadIdx, "index", m, "error", err)
			return
		}

		// append to the list of keys in this thread for future parent selection
		keys = append(keys, hash)
	}
}

// --- HTTP seeding helpers and types
type httpAuth struct {
	kvHashHex    string
	kvHashBase64 string
	browserKey   []byte
	client       *http.Client
}

// initHTTPAuth uses the given server's OTK and nonce to register a BrowserKey for later requests.
func initHTTPAuth(serverAddr, otkKeyB64, otkNonceB64 string) (*httpAuth, error) {
	// decode OTK
	otkKey, err := base64.StdEncoding.DecodeString(otkKeyB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode otk key: %w", err)
	}
	otkNonce, err := base64.StdEncoding.DecodeString(otkNonceB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode otk nonce: %w", err)
	}

	// Browser key: pick a random 32-byte key
	browserKey := make([]byte, 32)
	if _, err := crand.Read(browserKey); err != nil {
		return nil, fmt.Errorf("failed to generate browser key: %w", err)
	}

	// encrypt browserKey using OTK key/nonce with AES-GCM
	ciphertext, err := encryptAESGCM(otkKey, otkNonce, browserKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt browser key: %w", err)
	}

	reqBody := map[string]string{
		"data":   "b64:" + base64.StdEncoding.EncodeToString(ciphertext),
		"sha512": fmt.Sprintf("%x", sha512.Sum512(otkKey)),
	}
	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(serverAddr+"/authProcess", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to POST authProcess: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("authProcess request failed: status=%d body=%s", resp.StatusCode, string(b))
	}

	var respBody struct {
		Status    string `json:"status"`
		KeyKvHash string `json:"keyKvHash"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, fmt.Errorf("failed to decode authProcess response: %w", err)
	}

	// convert hex hash to raw bytes and base64 encode for header
	kvBytes, err := hex.DecodeString(respBody.KeyKvHash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode keyKvHash hex: %w", err)
	}
	kvBase64 := base64.StdEncoding.EncodeToString(kvBytes)

	return &httpAuth{kvHashHex: respBody.KeyKvHash, kvHashBase64: kvBase64, browserKey: browserKey, client: http.DefaultClient}, nil
}

// createThreadHTTP uses server API to create the thread and messages using the given auth.
func createThreadHTTP(serverAddr string, auth *httpAuth, threadIdx, messages int, articleTitle, lang string) {
	// if articleTitle provided, create thread from article via HTTP
	if articleTitle != "" {
		if err := createThreadHTTPFromArticle(serverAddr, auth, lang, articleTitle, messages); err != nil {
			slog.Error("failed to create thread from article (http)", "thread", threadIdx, "error", err)
		}
		return
	}
	// create root message
	rootContent := []byte(fmt.Sprintf("Thread %d root", threadIdx))
	rootKey, err := postDataHTTP(serverAddr, auth, rootContent, "text/plain; charset=utf-8", "", "", nil)
	if err != nil {
		slog.Error("failed to create thread root (http)", "thread", threadIdx, "error", err)
		return
	}

	var keys []string
	keys = append(keys, rootKey)
	maxDepth := rand.Intn(8) + 1

	for m := 1; m < messages; m++ {
		var parent string
		choice := rand.Intn(100)
		if choice < 60 {
			parent = keys[len(keys)-1]
		} else if choice < 90 {
			idx := len(keys) - 1 - rand.Intn(min(len(keys), maxDepth))
			parent = keys[idx]
		} else {
			parent = keys[rand.Intn(len(keys))]
		}

		var content []byte
		var mime string
		if rand.Intn(100) < 10 {
			content = []byte{0xde, 0xad, byte(threadIdx & 0xff), byte(m & 0xff)}
			mime = "application/octet-stream"
		} else {
			depth := computeDepthHTTP(keys, parent)
			content = []byte(fmt.Sprintf("Thread %d message %d depth %d parent %s", threadIdx, m, depth, parent))
			mime = "text/plain; charset=utf-8"
		}

		key, err := postDataHTTP(serverAddr, auth, content, mime, parent, "", nil)
		if err != nil {
			slog.Warn("failed to store message (http)", "thread", threadIdx, "index", m, "error", err)
			return
		}
		keys = append(keys, key)
	}
}

func computeDepthHTTP(keys []string, parent string) int {
	for i := len(keys) - 1; i >= 0; i-- {
		if keys[i] == parent {
			return len(keys) - 1 - i
		}
	}
	return 0
}

// headers := optional extra headers map
func postDataHTTP(serverAddr string, auth *httpAuth, content []byte, mime, parent, title string, headers map[string]string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", `form-data; name="file"; filename="payload"`)
	partHeader.Set("Content-Type", mime)
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(content); err != nil {
		return "", err
	}

	// metadata
	metadata := map[string]any{}
	if parent != "" {
		metadata["parent"] = parent
	}
	if title != "" {
		metadata["title"] = title
	}
	if len(metadata) > 0 {
		metaBytes, _ := json.Marshal(metadata)
		_ = writer.WriteField("metadata", string(metaBytes))
	}

	if err := writer.Close(); err != nil {
		return "", err
	}

	// construct token and nonce
	tokenPlain := []byte("auth:" + fmt.Sprintf("%d", time.Now().UnixNano()))
	tokenNonce := make([]byte, 12) // AES-GCM standard nonce
	if _, err := crand.Read(tokenNonce); err != nil {
		return "", err
	}
	tokenCipher, err := encryptAESGCM(auth.browserKey, tokenNonce, tokenPlain)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, serverAddr+"/data", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Auth-KeyHash-Base64", auth.kvHashBase64)
	req.Header.Set("X-Auth-Token", "b64:"+base64.StdEncoding.EncodeToString(tokenCipher))
	req.Header.Set("X-Auth-Nonce", "b64:"+base64.StdEncoding.EncodeToString(tokenNonce))
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := auth.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to POST /data: status=%d body=%s", resp.StatusCode, string(b))
	}
	var result struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Key, nil
}

func encryptAESGCM(key, nonce, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	a, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return a.Seal(nil, nonce, plaintext, nil), nil
}

func computeDepth(keys []cryptHash.Hash, parent cryptHash.Hash) int {
	for i := len(keys) - 1; i >= 0; i-- {
		if keys[i] == parent {
			return len(keys) - 1 - i
		}
	}
	return 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// createThreadFromArticle creates a thread and child messages from a Wikipedia article by language and title
func createThreadFromArticle(ctx context.Context, db *ouroboros.OuroborosDB, lang, title string, maxMessages int) error {
	sections, err := fetchArticleMobileSections(lang, title)
	if err != nil {
		return err
	}
	headingTitle := func(sectionTitle string) string {
		if strings.TrimSpace(sectionTitle) != "" {
			return sectionTitle
		}
		return title
	}

	// root content: use lead first paragraph if present
	rootContent := title
	if len(sections) > 0 && len(sections[0].Paragraphs) > 0 {
		rootContent = sections[0].Paragraphs[0]
	}
	// store root with title metadata
	rootHash, err := db.StoreData(ctx, []byte(rootContent), ouroboros.StoreOptions{MimeType: "text/plain; charset=utf-8", Title: title})
	if err != nil {
		return err
	}

	// map of level to parentHash for nested sections
	levelParents := make(map[int]cryptHash.Hash)
	levelParents[0] = rootHash

	// handle lead paragraphs (level 0) as children of root
	if len(sections) > 0 {
		leadTitle := headingTitle(sections[0].Title)
		for _, p := range sections[0].Paragraphs {
			if p == "" {
				continue
			}
			if maxMessages <= 0 {
				return nil
			}
			if _, err := db.StoreData(ctx, []byte(p), ouroboros.StoreOptions{Parent: rootHash, MimeType: "text/plain; charset=utf-8", Title: leadTitle}); err != nil {
				slog.Warn("failed to store paragraph", "title", title, "error", err)
			}
			maxMessages--
		}
	}

	// process remaining sections
	for i := 1; i < len(sections); i++ {
		s := sections[i]
		sectionTitle := headingTitle(s.Title)
		// find parent: nearest lower-level parent
		parent := rootHash
		for l := s.Level - 1; l >= 0; l-- {
			if h, ok := levelParents[l]; ok {
				parent = h
				break
			}
		}
		// create a section node using the title as the content and metadata
		sectionHash, err := db.StoreData(ctx, []byte(sectionTitle), ouroboros.StoreOptions{Parent: parent, MimeType: "text/plain; charset=utf-8", Title: sectionTitle})
		if err != nil {
			slog.Warn("failed to store section", "title", s.Title, "error", err)
			continue
		}
		levelParents[s.Level] = sectionHash
		// create paragraph nodes under this section node
		for _, p := range s.Paragraphs {
			if p == "" {
				continue
			}
			if maxMessages <= 0 {
				return nil
			}
			if _, err := db.StoreData(ctx, []byte(p), ouroboros.StoreOptions{Parent: sectionHash, MimeType: "text/plain; charset=utf-8", Title: sectionTitle}); err != nil {
				slog.Warn("failed to store paragraph", "section", s.Title, "error", err)
			}
			maxMessages--
		}
	}
	return nil
}

// createThreadHTTPFromArticle creates thread and child messages via HTTP posts using given auth
func createThreadHTTPFromArticle(serverAddr string, auth *httpAuth, lang, title string, maxMessages int) error {
	sections, err := fetchArticleMobileSections(lang, title)
	if err != nil {
		return err
	}
	headingTitle := func(sectionTitle string) string {
		if strings.TrimSpace(sectionTitle) != "" {
			return sectionTitle
		}
		return title
	}
	// root content: first lead paragraph or the title
	rootContent := title
	if len(sections) > 0 && len(sections[0].Paragraphs) > 0 {
		rootContent = sections[0].Paragraphs[0]
	}
	rootKey, err := postDataHTTP(serverAddr, auth, []byte(rootContent), "text/plain; charset=utf-8", "", title, nil)
	if err != nil {
		return err
	}
	levelParents := make(map[int]string)
	// level 0 is root
	levelParents[0] = rootKey
	// lead paragraphs (level 0)
	if len(sections) > 0 {
		leadTitle := headingTitle(sections[0].Title)
		for _, p := range sections[0].Paragraphs {
			if p == "" {
				continue
			}
			if maxMessages <= 0 {
				return nil
			}
			if _, err := postDataHTTP(serverAddr, auth, []byte(p), "text/plain; charset=utf-8", rootKey, leadTitle, nil); err != nil {
				slog.Warn("failed to post paragraph", "title", title, "error", err)
			}
			maxMessages--
		}
	}
	for i := 1; i < len(sections); i++ {
		s := sections[i]
		sectionTitle := headingTitle(s.Title)
		parent := rootKey
		for l := s.Level - 1; l >= 0; l-- {
			if h, ok := levelParents[l]; ok {
				parent = h
				break
			}
		}
		sectionKey, err := postDataHTTP(serverAddr, auth, []byte(sectionTitle), "text/plain; charset=utf-8", parent, sectionTitle, nil)
		if err != nil {
			slog.Warn("failed to post section", "title", s.Title, "error", err)
			continue
		}
		levelParents[s.Level] = sectionKey
		for _, p := range s.Paragraphs {
			if p == "" {
				continue
			}
			if maxMessages <= 0 {
				return nil
			}
			if _, err := postDataHTTP(serverAddr, auth, []byte(p), "text/plain; charset=utf-8", sectionKey, sectionTitle, nil); err != nil {
				slog.Warn("failed to post paragraph", "section", s.Title, "error", err)
			}
			maxMessages--
		}
	}
	return nil
}
