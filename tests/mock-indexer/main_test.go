package main_test

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	mockindexer "github.com/bruceowenga/arrstart/tests/mock-indexer"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(mockindexer.New().Handler())
}

func TestCaps(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api?t=caps")
	if err != nil {
		t.Fatalf("GET caps: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read caps: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("caps status = %d, want %d: %s", resp.StatusCode, http.StatusOK, body)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/xml") {
		t.Errorf("caps Content-Type = %q, want application/xml prefix", got)
	}
	if !strings.Contains(string(body), `<caps>`) {
		t.Errorf("caps missing <caps> element: %s", body)
	}
	if !strings.Contains(string(body), "tvsearch") || !strings.Contains(string(body), "movie") {
		t.Errorf("caps missing tvsearch/movie limits: %s", body)
	}
	if !strings.Contains(string(body), "Mock Indexer") {
		t.Errorf("caps missing title: %s", body)
	}
}

func TestTVSearch(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api?t=tvsearch&cat=5000&q=mock.show")
	if err != nil {
		t.Fatalf("GET tvsearch: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read tvsearch: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tvsearch status = %d, want %d: %s", resp.StatusCode, http.StatusOK, body)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/xml") {
		t.Errorf("tvsearch Content-Type = %q, want application/xml prefix", got)
	}
	if !strings.Contains(string(body), `<rss`) {
		t.Errorf("tvsearch missing <rss> root: %s", body)
	}
	if !strings.Contains(string(body), `xmlns:torznab="http://torznab.com/schemas/2015/manifest"`) {
		t.Errorf("tvsearch missing torznab namespace declaration on <rss>: %s", body)
	}
	if !strings.Contains(string(body), `xmlns="http://torznab.com/schemas/2015/manifest"`) {
		t.Errorf("tvsearch missing namespaced torznab attr elements: %s", body)
	}
	if !strings.Contains(string(body), "mock.show.s01e01") {
		t.Errorf("tvsearch missing expected fake result: %s", body)
	}
	// "mock.show" matches only s01e01 of the two TV candidates.
	if got := strings.Count(string(body), "<item>"); got != 1 {
		t.Errorf("tvsearch item count = %d, want 1 for q=mock.show: %s", got, body)
	}
	// No real indexer names or content are ever served (ADR-0009).
	if strings.Contains(string(body), "thepiratebay") || strings.Contains(string(body), "1337x") {
		t.Errorf("tvsearch references a real indexer: %s", body)
	}
}

func TestMovieSearch(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api?t=movie&cat=2000&q=mock.movie")
	if err != nil {
		t.Fatalf("GET movie: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read movie: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("movie status = %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "mock.movie.2026.1080p") {
		t.Errorf("movie missing expected fake result: %s", body)
	}
	// q=mock.movie matches only the 2026 title of the two movie candidates.
	if got := strings.Count(string(body), "<item>"); got != 1 {
		t.Errorf("movie item count = %d, want 1 for q=mock.movie: %s", got, body)
	}
}

func TestSearchNoCatReturnsTVAndMovie(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api?t=search&q=mock")
	if err != nil {
		t.Fatalf("GET search: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read search: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search status = %d: %s", resp.StatusCode, body)
	}
	// No category ⇒ search spans TV and movie: 2 TV + 2 movie candidates match q=mock.
	if got := strings.Count(string(body), "<item>"); got != 4 {
		t.Errorf("search(no cat) item count = %d, want 4 (TV + movie): %s", got, body)
	}
	if !strings.Contains(string(body), "mock.show.s01e01") || !strings.Contains(string(body), "mock.tv.s02e03") ||
		!strings.Contains(string(body), "mock.movie.2026.1080p") || !strings.Contains(string(body), "mock.film.2025") {
		t.Errorf("search(no cat) missing expected results: %s", body)
	}
}

func TestSearchUnmatchedQueryReturnsEmptyChannel(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api?t=tvsearch&cat=5000&q=zzz-no-match")
	if err != nil {
		t.Fatalf("GET search: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read search: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search status = %d: %s", resp.StatusCode, body)
	}
	// Well-formed RSS with an empty channel (zero items) is valid.
	if !strings.Contains(string(body), `<channel>`) || !strings.Contains(string(body), `</channel>`) {
		t.Errorf("search empty result missing channel: %s", body)
	}
}

func TestCapsXMLWellFormed(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api?t=caps")
	if err != nil {
		t.Fatalf("GET caps: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read caps: %v", err)
	}
	var caps struct {
		XMLName xml.Name `xml:"caps"`
		Title   string   `xml:"title"`
	}
	if err := xml.Unmarshal(body, &caps); err != nil {
		t.Fatalf("caps not well-formed XML: %v", err)
	}
	if caps.Title != "Mock Indexer" {
		t.Errorf("caps Title = %q, want %q", caps.Title, "Mock Indexer")
	}
}

func TestSearchXMLWellFormed(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api?t=search&cat=5000")
	if err != nil {
		t.Fatalf("GET search: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read search: %v", err)
	}
	var rss struct {
		XMLName xml.Name `xml:"rss"`
	}
	if err := xml.Unmarshal(body, &rss); err != nil {
		t.Fatalf("search not well-formed XML: %v", err)
	}
	// encoding/xml auto-declares a namespace (prefix may vary) for the
	// torznab:attr elements so real parsers don't reject an undeclared prefix.
	if !regexp.MustCompile(`xmlns:[a-zA-Z_]+="http://torznab\.com/schemas/2015/manifest"`).MatchString(string(body)) {
		t.Errorf("search missing torznab namespace declaration: %s", body)
	}
}
