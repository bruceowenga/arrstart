// Command mock-indexer is a throwaway Torznab indexer for Gate 3 e2e
// verification (BUILD-PLAN §5.2). It implements the Torznab /api surface
// (caps, tv/movie search) and returns only obviously-fake results — provider
// "mock-indexer", fake titles such as "mock.show.s01e01", fake IMDb/TVDB IDs.
// No real indexer names or content are ever served (repo rule, ADR-0009).
//
// It runs standalone (for manual verification against a real Prowlarr) and as
// a container (tests/mock-indexer/Dockerfile). The handler is exported so unit
// tests can drive it with httptest without binding a real port.
package main

import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Torznab XML namespace (declared once on the <rss> root element).
const torznabNS = "http://torznab.com/schemas/2015/manifest"

// Newznab/Torznab category IDs (extended scheme). cat absent ⇒ TV results.
var tvCats = map[string]bool{
	"5000": true, "5010": true, "5020": true, "5030": true,
	"5040": true, "5050": true, "5060": true, "5070": true,
}
var movieCats = map[string]bool{
	"2000": true, "2010": true, "2020": true, "2030": true,
	"2040": true, "2050": true, "2060": true, "2070": true,
}

// MockIndexer is the Torznab mock indexer handler.
type MockIndexer struct{}

// New returns a mock indexer that serves obviously-fake Torznab results.
func New() *MockIndexer { return &MockIndexer{} }

// Handler returns an http.Handler implementing the Torznab /api surface.
func (m *MockIndexer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api", m.handleAPI)
	return mux
}

func isTV(cats []string) bool   { return anyCat(cats, tvCats) }
func isMovie(cats []string) bool { return anyCat(cats, movieCats) }

func anyCat(cats []string, set map[string]bool) bool {
	for _, c := range cats {
		if set[strings.TrimSpace(c)] {
			return true
		}
	}
	return false
}

func (m *MockIndexer) handleAPI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	t := q.Get("t")
	cats := strings.Split(q.Get("cat"), ",")

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")

	switch t {
	case "caps":
		writeXML(w, http.StatusOK, capsXML())
	case "tvsearch":
		writeXML(w, http.StatusOK, searchXML(tvResults(cats, q)))
	case "movie":
		writeXML(w, http.StatusOK, searchXML(movieResults(cats, q)))
	case "search":
		items := tvResults(cats, q)
		if isMovie(cats) && !isTV(cats) {
			items = movieResults(cats, q)
		} else if isMovie(cats) || len(cats) == 1 && cats[0] == "" {
			// Explicit movie category, or no category at all: include both.
			items = append(tvResults(cats, q), movieResults(cats, q)...)
		}
		writeXML(w, http.StatusOK, searchXML(items))
	default:
		http.Error(w, "unknown torznab command", http.StatusBadRequest)
	}
}

// tvResults returns obviously-fake TV results filtered by the search query q.
func tvResults(cats []string, q url.Values) []searchItem {
	if !isTV(cats) && len(cats) > 0 && cats[0] != "" {
		return nil
	}
	cands := []searchItem{
		{title: "mock.show.s01e01", guid: "mock-indexer:tv:mock.show.s01e01",
			imdb: "tt0000001", tvdb: "000001", cat: "5050", size: 700000000, seeders: 10, peers: 2},
		{title: "mock.tv.s02e03", guid: "mock-indexer:tv:mock.tv.s02e03",
			imdb: "tt0000002", tvdb: "000002", cat: "5030", size: 1400000000, seeders: 5, peers: 1},
	}
	return filterByQuery(cands, q.Get("q"))
}

// movieResults returns obviously-fake movie results filtered by the search query q.
func movieResults(cats []string, q url.Values) []searchItem {
	if !isMovie(cats) && len(cats) > 0 && cats[0] != "" {
		return nil
	}
	cands := []searchItem{
		{title: "mock.movie.2026.1080p", guid: "mock-indexer:movie:mock.movie.2026.1080p",
			imdb: "tt0000010", tvdb: "", cat: "2050", size: 4500000000, seeders: 20, peers: 4},
		{title: "mock.film.2025", guid: "mock-indexer:movie:mock.film.2025",
			imdb: "tt0000011", tvdb: "", cat: "2050", size: 2200000000, seeders: 8, peers: 3},
	}
	return filterByQuery(cands, q.Get("q"))
}

func filterByQuery(items []searchItem, q string) []searchItem {
	if q == "" {
		return items
	}
	q = strings.ToLower(q)
	out := make([]searchItem, 0, len(items))
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.title), q) {
			out = append(out, it)
		}
	}
	return out
}

// searchItem is a fake Torznab result.
type searchItem struct {
	title string
	guid  string
	imdb  string
	tvdb  string
	cat   string // Newznab category ID as a string
	size  int64
	seeders int
	peers   int
}

func searchXML(items []searchItem) []byte {
	v := struct {
		XMLName xml.Name `xml:"rss"`
		Version string   `xml:"version,attr"`
		NS      string   `xml:"xmlns:torznab,attr"`
		Channel channel  `xml:"channel"`
	}{
		Version: "2.0",
		NS:      torznabNS,
		Channel: channel{
			Title:     "Mock Indexer",
			Link:      "http://mock-indexer:9999",
			Description: "Mock Torznab indexer for arrstart Gate 3 e2e",
			Items:     itemsToXML(items),
		},
	}
	out, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Printf("mock-indexer: marshal search XML: %v", err)
		return nil
	}
	return append([]byte(xml.Header), out...)
}

func itemsToXML(items []searchItem) []channelItem {
	out := make([]channelItem, 0, len(items))
	now := "Mon, 02 Nov 2026 12:00:00 +0000"
	for _, it := range items {
		attrs := []attr{
			{Name: "size", Value: strconv.FormatInt(it.size, 10)},
			{Name: "seeders", Value: strconv.Itoa(it.seeders)},
			{Name: "peers", Value: strconv.Itoa(it.peers)},
			{Name: "downloadvolumefactor", Value: "1.0"},
			{Name: "uploadvolumefactor", Value: "1.0"},
			{Name: "imdb", Value: it.imdb},
			{Name: "tvdb", Value: it.tvdb},
			{Name: "type", Value: "tv-search"},
			{Name: "category", Value: it.cat},
			{Name: "downloadurl", Value: fmt.Sprintf("http://mock-indexer:9999/download/%s.torrent", strings.ReplaceAll(it.title, " ", "_"))},
		}
		out = append(out, channelItem{
			Title:   it.title,
			GUID:    it.guid,
			Link:    fmt.Sprintf("http://mock-indexer:9999/view/%s", it.guid),
			PubDate: now,
			Attrs:   attrs,
		})
	}
	return out
}

type channelItem struct {
	XMLName xml.Name `xml:"item"`
	Title   string   `xml:"title"`
	GUID    string   `xml:"guid"`
	Link    string   `xml:"link"`
	PubDate string   `xml:"pubDate"`
	Attrs   []attr   `xml:"torznab:attr"`
}

type attr struct {
	XMLName xml.Name `xml:"http://torznab.com/schemas/2015/manifest attr"`
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type channel struct {
	XMLName     xml.Name `xml:"channel"`
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	Description string   `xml:"description"`
	Items       []channelItem `xml:"item"`
}

func capsXML() []byte {
	v := struct {
		XMLName xml.Name `xml:"caps"`
		Title   string   `xml:"title"`
		Server  string   `xml:"server"`
		Desc    string   `xml:"description"`
		Link    string   `xml:"link"`
		Limits  limits   `xml:"limits"`
	}{
		Title:      "Mock Indexer",
		Server:     "",
		Desc:       "Mock Torznab indexer for arrstart Gate 3 e2e",
		Link:       "http://mock-indexer:9999",
		Limits: limits{
			TVSearch:   limit{Name: "TV Search", Enabled: true, SupportedParams: "q,season,ep"},
			Movie:      limit{Name: "Movie Search", Enabled: true, SupportedParams: "q"},
			Search:     limit{Name: "Search", Enabled: true, SupportedParams: "q,cat"},
		},
	}
	out, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Printf("mock-indexer: marshal caps XML: %v", err)
		return nil
	}
	return append([]byte(xml.Header), out...)
}

type limits struct {
	XMLName xml.Name `xml:"limits"`
	TVSearch limit `xml:"tvsearch"`
	Movie   limit `xml:"movie"`
	Search  limit `xml:"search"`
}

type limit struct {
	Name          string `xml:"name,attr"`
	Enabled       bool   `xml:"enabled,attr"`
	SupportedParams string `xml:"supportedParams,attr"`
}

func writeXML(w http.ResponseWriter, status int, b []byte) {
	if b == nil {
		http.Error(w, "mock-indexer: internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(status)
	if _, err := w.Write(b); err != nil {
		log.Printf("mock-indexer: write response: %v", err)
	}
}

func main() {
	port := os.Getenv("MOCK_INDEXER_PORT")
	if port == "" {
		port = "9999"
	}
	addr := ":" + port
	if h := os.Getenv("MOCK_INDEXER_HOST"); h != "" {
		addr = h + ":" + port
	}
	srv := &http.Server{
		Addr:    addr,
		Handler: New().Handler(),
	}
	log.Printf("mock-indexer: listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("mock-indexer: %v", err)
	}
}
