package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cryptHash "github.com/i5heu/ouroboros-crypt/pkg/hash"
	keys "github.com/i5heu/ouroboros-crypt/pkg/keys"
	ouroboros "github.com/i5heu/ouroboros-db"
)

// helper to rewrite requests for wikipedia hosts to the test server
type rewriteTransport struct {
	base   http.RoundTripper
	target string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// rewrite wiki host requests
	if strings.Contains(req.URL.Host, "wikipedia.org") {
		// preserve path and query
		new := *req
		// replace scheme and host
		new.URL.Scheme = "http"
		new.URL.Host = t.target
		new.Host = t.target
		return t.base.RoundTrip(&new)
	}
	return t.base.RoundTrip(req)
}

func TestCreateThreadFromArticle_DB(t *testing.T) {
	// start wiki mock server
	wiki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/random/title"):
			// return single item list
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"title":"Test Article DB"}]}`))
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/mobile-html/"):
			w.Header().Set("Content-Type", "text/html")
			html := `<html><body>
	<section data-mw-section-id="0">
		<p>Lead paragraph DB</p>
	</section>
	<section data-mw-section-id="1">
		<h2>Section A</h2>
		<p>Para A1</p>
		<p>Para A2</p>
	</section>
	</body></html>`
			_, _ = w.Write([]byte(html))
		default:
			http.NotFound(w, r)
		}
	}))
	defer wiki.Close()

	// rewrite default transport to point wiki requests to our server
	origTransport := http.DefaultTransport
	http.DefaultTransport = &rewriteTransport{base: origTransport, target: strings.TrimPrefix(wiki.Listener.Addr().String(), "")}
	defer func() { http.DefaultTransport = origTransport }()

	// create temp DB
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "ouroboros.key")
	a, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("new async crypt: %v", err)
	}
	if err := a.SaveToFile(keyPath); err != nil {
		t.Fatalf("save keyfile: %v", err)
	}

	db, err := ouroboros.New(ouroboros.Config{Paths: []string{dir}, MinimumFreeGB: 1})
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	if err := db.Start(context.Background()); err != nil {
		t.Fatalf("start db: %v", err)
	}
	defer func() { _ = db.CloseWithoutContext() }()

	// set limiter for tests to 10 rps to be safe
	wikiLimiter = newRateLimiter(10)

	// create thread from mock article
	if err := createThreadFromArticle(context.Background(), db, "de", "Test Article DB", 10); err != nil {
		t.Fatalf("createThreadFromArticle: %v", err)
	}

	// verify root and children exist
	keysList, err := db.ListData(context.Background())
	if err != nil {
		t.Fatalf("list data: %v", err)
	}
	if len(keysList) == 0 {
		t.Fatalf("expected at least one root key")
	}
	// check first root has matching Title and a child
	root := keysList[0]
	data, err := db.GetData(context.Background(), root)
	if err != nil {
		t.Fatalf("get data: %v", err)
	}
	if data.Title != "Test Article DB" {
		t.Fatalf("expected title %q, got %q", "Test Article DB", data.Title)
	}
	children, err := db.ListChildren(context.Background(), root)
	if err != nil {
		t.Fatalf("get children: %v", err)
	}
	if len(children) == 0 {
		t.Fatalf("expected at least one child under root")
	}
	leadParagraphFound := false
	var sectionNode cryptHash.Hash
	sectionFound := false
	for _, child := range children {
		childData, err := db.GetData(context.Background(), child)
		if err != nil {
			t.Fatalf("get child data: %v", err)
		}
		if childData.Title == "Test Article DB" && strings.Contains(string(childData.Content), "Lead paragraph DB") {
			leadParagraphFound = true
		}
		if childData.Title == "Section A" && string(childData.Content) == "Section A" {
			sectionNode = child
			sectionFound = true
		}
	}
	if !leadParagraphFound {
		t.Fatalf("expected a lead paragraph child with title metadata")
	}
	if !sectionFound {
		t.Fatalf("expected to find stored section node")
	}
	sectionChildren, err := db.ListChildren(context.Background(), sectionNode)
	if err != nil {
		t.Fatalf("list section children: %v", err)
	}
	sectionParagraphFound := false
	for _, child := range sectionChildren {
		childData, err := db.GetData(context.Background(), child)
		if err != nil {
			t.Fatalf("get section child data: %v", err)
		}
		if childData.Title == "Section A" && strings.Contains(string(childData.Content), "Para A1") {
			sectionParagraphFound = true
			break
		}
	}
	if !sectionParagraphFound {
		t.Fatalf("expected section paragraph child to inherit section title metadata")
	}
}

func TestCreateThreadFromArticleHTTP(t *testing.T) {
	// wiki mock server
	wiki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/random/title"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"title":"Test Article HTTP"}]}`))
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/mobile-html/"):
			w.Header().Set("Content-Type", "text/html")
			html := `<html><body>
	<section data-mw-section-id="0">
		<p>Lead paragraph HTTP</p>
	</section>
	<section data-mw-section-id="1">
		<h2>Section B</h2>
		<p>Para B1</p>
		<p>Para B2</p>
	</section>
	</body></html>`
			_, _ = w.Write([]byte(html))
		default:
			http.NotFound(w, r)
		}
	}))
	defer wiki.Close()

	// rewrite transport for wiki to point to server
	origTransport := http.DefaultTransport
	http.DefaultTransport = &rewriteTransport{base: origTransport, target: strings.TrimPrefix(wiki.Listener.Addr().String(), "")}
	defer func() { http.DefaultTransport = origTransport }()

	// HTTP server to accept POST /data
	posted := make([]map[string]any, 0)
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/data" {
			// parse multipart
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			meta := r.FormValue("metadata")
			var parsed map[string]any
			if meta != "" {
				json.Unmarshal([]byte(meta), &parsed)
			}
			// store recorded metadata
			posted = append(posted, parsed)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"key":"dummykey` + time.Now().Format("150405") + `"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer apiServer.Close()

	// set limiter
	wikiLimiter = newRateLimiter(10)

	auth := &httpAuth{kvHashBase64: "test", browserKey: make([]byte, 32), client: &http.Client{}}

	// call helper to create thread via HTTP
	if err := createThreadHTTPFromArticle(apiServer.URL, auth, "de", "Test Article HTTP", 10); err != nil {
		t.Fatalf("createThreadHTTPFromArticle: %v", err)
	}

	// ensure some posts had metadata title for the root or sections
	foundRootTitle := false
	foundSectionTitle := false
	for _, p := range posted {
		if p == nil {
			continue
		}
		if t, ok := p["title"].(string); ok {
			switch t {
			case "Test Article HTTP":
				foundRootTitle = true
			case "Section B":
				foundSectionTitle = true
			}
		}
	}
	if !foundRootTitle {
		t.Fatalf("expected to see a POST with root title metadata, got %+v", posted)
	}
	if !foundSectionTitle {
		t.Fatalf("expected to see a POST with section title metadata for paragraphs, got %+v", posted)
	}
}

// Test that runs the mockData main() in DB mode, using a wiki mock server
// and a temporary data directory. This ensures the CLI mode works end-to-end.
func TestMockDataMain_DB_Run(t *testing.T) {
	// wiki mock server
	wiki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/random/title"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"title":"Test Main Article DB"}]}`))
			return
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/mobile-html/"):
			w.Header().Set("Content-Type", "text/html")
			html := `<html><body>
<section data-mw-section-id="0">
	<p>Lead main paragraph</p>
</section>
<section data-mw-section-id="1">
	<h2>Section M</h2>
	<p>Para M1</p>
</section>
</body></html>`
			_, _ = w.Write([]byte(html))
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer wiki.Close()

	// rewrite default transport to point wiki requests to our server
	origTransport := http.DefaultTransport
	http.DefaultTransport = &rewriteTransport{base: origTransport, target: strings.TrimPrefix(wiki.Listener.Addr().String(), "")}
	defer func() { http.DefaultTransport = origTransport }()

	// tmp data dir to be used by main
	dir := t.TempDir()

	args := []string{"-threads=1", "-messages=2", "-path=" + dir, "-wiki-rps=10", "-wiki-lang=de"}
	if err := run(args); err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	// Open DB and verify contents
	db, err := ouroboros.New(ouroboros.Config{Paths: []string{dir}, MinimumFreeGB: 1})
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	if err := db.Start(context.Background()); err != nil {
		t.Fatalf("start db: %v", err)
	}
	defer func() { _ = db.CloseWithoutContext() }()

	keysList, err := db.ListData(context.Background())
	if err != nil {
		t.Fatalf("list data: %v", err)
	}
	if len(keysList) == 0 {
		t.Fatalf("expected at least one root key after running main")
	}
	root := keysList[0]
	data, err := db.GetData(context.Background(), root)
	if err != nil {
		t.Fatalf("get data: %v", err)
	}
	if data.Title != "Test Main Article DB" {
		t.Fatalf("expected title %q, got %q", "Test Main Article DB", data.Title)
	}
}

func TestMockDataMain_DB_Run_UserAgent(t *testing.T) {
	// wiki mock server that requires User-Agent header
	wiki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("User-Agent"), "ouroboros-mockData") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("Please set a user-agent"))
			return
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/random/title"):
			w.Header().Set("Location", "/api/rest_v1/page/title/Test UA Article DB")
			w.WriteHeader(http.StatusSeeOther)
			return
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/title/"):
			w.Header().Set("Content-Type", "application/json")
			resp := `{"items":[{"title":"Test UA Article DB"}]}`
			_, _ = w.Write([]byte(resp))
			return
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/mobile-html/"):
			w.Header().Set("Content-Type", "text/html")
			html := `<html><body>
	<section data-mw-section-id="0">
		<p>UA paragraph</p>
	</section>
	</body></html>`
			_, _ = w.Write([]byte(html))
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer wiki.Close()

	origTransport := http.DefaultTransport
	http.DefaultTransport = &rewriteTransport{base: origTransport, target: strings.TrimPrefix(wiki.Listener.Addr().String(), "")}
	defer func() { http.DefaultTransport = origTransport }()

	dir := t.TempDir()
	args := []string{"-threads=1", "-messages=2", "-path=" + dir, "-wiki-rps=10", "-wiki-lang=de"}
	if err := run(args); err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	db, err := ouroboros.New(ouroboros.Config{Paths: []string{dir}, MinimumFreeGB: 1})
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	if err := db.Start(context.Background()); err != nil {
		t.Fatalf("start db: %v", err)
	}
	defer func() { _ = db.CloseWithoutContext() }()

	keysList, err := db.ListData(context.Background())
	if err != nil {
		t.Fatalf("list data: %v", err)
	}
	if len(keysList) == 0 {
		t.Fatalf("expected at least one root key after running main with UA requirement")
	}
	root := keysList[0]
	data, err := db.GetData(context.Background(), root)
	if err != nil {
		t.Fatalf("get data: %v", err)
	}
	if data.Title != "Test UA Article DB" {
		t.Fatalf("expected title %q, got %q", "Test UA Article DB", data.Title)
	}
}

// Test that fetchRandomWikipediaTitles handles HTTP redirects correctly.
// The real Wikipedia API returns 303 redirects, not JSON directly.
func TestFetchRandomWikipediaTitles_WithRedirect(t *testing.T) {
	// Mock server that mimics real Wikipedia behavior with redirects
	wiki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/random/title"):
			if !strings.Contains(r.Header.Get("User-Agent"), "ouroboros-mockData") {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("Please set a user-agent"))
				return
			}
			// Real Wikipedia returns a 303 redirect to the actual page title
			w.Header().Set("Location", "/api/rest_v1/page/title/Test_Redirect_Article")
			w.WriteHeader(http.StatusSeeOther)
			_, _ = w.Write([]byte("See Other. Redirecting to /api/rest_v1/page/title/Test_Redirect_Article"))
			return
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/title/"):
			// The redirected endpoint should return JSON with the title
			parts := strings.Split(r.URL.Path, "/")
			title := parts[len(parts)-1]
			// Unescape underscores to spaces
			title = strings.ReplaceAll(title, "_", " ")
			w.Header().Set("Content-Type", "application/json")
			resp := `{"items":[{"title":"` + title + `"}]}`
			_, _ = w.Write([]byte(resp))
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer wiki.Close()

	// rewrite default transport to point wiki requests to our server
	origTransport := http.DefaultTransport
	http.DefaultTransport = &rewriteTransport{base: origTransport, target: strings.TrimPrefix(wiki.Listener.Addr().String(), "")}
	defer func() { http.DefaultTransport = origTransport }()

	// set limiter
	wikiLimiter = newRateLimiter(10)

	titles, err := fetchRandomWikipediaTitles("de", 1)
	if err != nil {
		t.Fatalf("fetchRandomWikipediaTitles failed: %v", err)
	}
	if len(titles) != 1 {
		t.Fatalf("expected 1 title, got %d", len(titles))
	}
	if titles[0] != "Test Redirect Article" {
		t.Fatalf("expected title %q, got %q", "Test Redirect Article", titles[0])
	}
}

func TestFetchRandomWikipediaTitles_BadIntegerFallback(t *testing.T) {
	wiki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/random/title"):
			if !strings.Contains(r.Header.Get("User-Agent"), "ouroboros-mockData") {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("Please set a user-agent"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"parameter-validation-failed","name":"id","value":"Reiten","failureCode":"badinteger","errorKey":"badinteger"}`))
			return
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/title/Reiten"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"title":"Reiten"}]}`))
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer wiki.Close()

	origTransport := http.DefaultTransport
	http.DefaultTransport = &rewriteTransport{base: origTransport, target: strings.TrimPrefix(wiki.Listener.Addr().String(), "")}
	defer func() { http.DefaultTransport = origTransport }()

	wikiLimiter = newRateLimiter(10)

	titles, err := fetchRandomWikipediaTitles("de", 1)
	if err != nil {
		t.Fatalf("fetchRandomWikipediaTitles failed: %v", err)
	}
	if len(titles) != 1 || titles[0] != "Reiten" {
		t.Fatalf("expected fallback title 'Reiten', got %+v", titles)
	}
}
