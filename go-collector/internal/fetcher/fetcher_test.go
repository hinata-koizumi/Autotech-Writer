package fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/autotech-writer/go-collector/internal/models"
	"github.com/mmcdole/gofeed"
)

// ============================================================
// arXiv XML パースロジックテスト
// ============================================================

// [正常系] arXivのXMLを共通構造体に正しくマッピングできること
func TestParseArxivXML_Normal(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/2401.00001v1</id>
    <title>Attention Is All You Need: Revisited</title>
    <summary>We revisit the transformer architecture and propose improvements.</summary>
    <published>2024-01-15T00:00:00Z</published>
    <link href="http://arxiv.org/abs/2401.00001v1" rel="alternate" type="text/html"/>
    <author><name>Test Author</name></author>
  </entry>
  <entry>
    <id>http://arxiv.org/abs/2401.00002v1</id>
    <title>Scaling Laws for LLMs</title>
    <summary>An analysis of scaling behavior in large language models.</summary>
    <published>2024-01-16T12:00:00Z</published>
    <link href="http://arxiv.org/abs/2401.00002v1" rel="alternate" type="text/html"/>
    <author><name>Another Author</name></author>
  </entry>
</feed>`)

	items, err := ParseArxivXML(xmlData)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Verify first item
	item := items[0]
	if item.SourceType != "arxiv" {
		t.Errorf("expected SourceType 'arxiv', got '%s'", item.SourceType)
	}
	if item.SourceID != "http://arxiv.org/abs/2401.00001v1" {
		t.Errorf("expected SourceID 'http://arxiv.org/abs/2401.00001v1', got '%s'", item.SourceID)
	}
	if item.Title != "Attention Is All You Need: Revisited" {
		t.Errorf("expected title 'Attention Is All You Need: Revisited', got '%s'", item.Title)
	}
	if item.Summary != "We revisit the transformer architecture and propose improvements." {
		t.Errorf("unexpected summary: %s", item.Summary)
	}
	if item.URL != "http://arxiv.org/abs/2401.00001v1" {
		t.Errorf("expected URL 'http://arxiv.org/abs/2401.00001v1', got '%s'", item.URL)
	}
	expectedTime, _ := time.Parse(time.RFC3339, "2024-01-15T00:00:00Z")
	if !item.PublishedAt.Equal(expectedTime) {
		t.Errorf("expected PublishedAt %v, got %v", expectedTime, item.PublishedAt)
	}
}

// [異常系] 不正なXMLでもパニックせずエラーを返すこと
func TestParseArxivXML_InvalidXML(t *testing.T) {
	invalidXML := []byte(`<this is not valid xml!!! <><>`)

	_, err := ParseArxivXML(invalidXML)
	if err == nil {
		t.Fatal("expected error for invalid XML, got nil")
	}
}

// [異常系] 必須フィールド（ID）が空のエントリはスキップすること
func TestParseArxivXML_EmptyID(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id></id>
    <title>Paper Without ID</title>
    <summary>This should be skipped.</summary>
    <published>2024-01-15T00:00:00Z</published>
  </entry>
  <entry>
    <id>http://arxiv.org/abs/2401.00003v1</id>
    <title>Valid Paper</title>
    <summary>This should be included.</summary>
    <published>2024-01-15T00:00:00Z</published>
  </entry>
</feed>`)

	items, err := ParseArxivXML(xmlData)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item (empty ID skipped), got %d", len(items))
	}
	if items[0].SourceID != "http://arxiv.org/abs/2401.00003v1" {
		t.Errorf("expected valid paper ID, got '%s'", items[0].SourceID)
	}
}

// [異常系] 日付フォーマットが想定外でもクラッシュせずゼロ値を設定すること
func TestParseArxivXML_BadDateFormat(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/2401.00004v1</id>
    <title>Paper with Bad Date</title>
    <summary>Test summary.</summary>
    <published>January 15, 2024</published>
  </entry>
</feed>`)

	items, err := ParseArxivXML(xmlData)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	if !items[0].PublishedAt.IsZero() {
		t.Errorf("expected zero time for bad date format, got %v", items[0].PublishedAt)
	}
}

// [正常系] 空のフィードは空のスライスを返すこと
func TestParseArxivXML_EmptyFeed(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
</feed>`)

	items, err := ParseArxivXML(xmlData)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items for empty feed, got %d", len(items))
	}
}

// ============================================================
// GitHub JSON パースロジックテスト
// ============================================================

// [正常系] GitHub ReleaseのJSONを共通構造体に正しくマッピングできること
func TestParseGitHubJSON_Normal(t *testing.T) {
	releases := []GitHubRelease{
		{
			ID:          1,
			TagName:     "v1.0.0",
			Name:        "Version 1.0.0",
			Body:        "Initial release with major features.",
			HTMLURL:     "https://github.com/org/repo/releases/tag/v1.0.0",
			PublishedAt: "2024-01-20T10:00:00Z",
		},
		{
			ID:          2,
			TagName:     "v1.1.0",
			Name:        "Version 1.1.0",
			Body:        "Bug fixes and performance improvements.",
			HTMLURL:     "https://github.com/org/repo/releases/tag/v1.1.0",
			PublishedAt: "2024-02-15T14:30:00Z",
		},
	}
	data, _ := json.Marshal(releases)

	items, err := ParseGitHubJSON(data, "org/repo")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	item := items[0]
	if item.SourceType != "github" {
		t.Errorf("expected SourceType 'github', got '%s'", item.SourceType)
	}
	if item.SourceID != "github:org/repo:v1.0.0" {
		t.Errorf("expected SourceID 'github:org/repo:v1.0.0', got '%s'", item.SourceID)
	}
	if item.Title != "Version 1.0.0" {
		t.Errorf("expected title 'Version 1.0.0', got '%s'", item.Title)
	}
	if item.Summary != "Initial release with major features." {
		t.Errorf("unexpected summary: %s", item.Summary)
	}
	if item.URL != "https://github.com/org/repo/releases/tag/v1.0.0" {
		t.Errorf("unexpected URL: %s", item.URL)
	}
}

// [異常系] 不正なJSONでもパニックせずエラーを返すこと
func TestParseGitHubJSON_InvalidJSON(t *testing.T) {
	_, err := ParseGitHubJSON([]byte(`{invalid json`), "org/repo")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// [異常系] tag_nameが空のリリースはスキップすること
func TestParseGitHubJSON_EmptyTagName(t *testing.T) {
	releases := []GitHubRelease{
		{ID: 1, TagName: "", Name: "No Tag", Body: "Should be skipped"},
		{ID: 2, TagName: "v2.0.0", Name: "Valid", Body: "Should be included", HTMLURL: "https://example.com", PublishedAt: "2024-01-01T00:00:00Z"},
	}
	data, _ := json.Marshal(releases)

	items, err := ParseGitHubJSON(data, "org/repo")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (empty tag skipped), got %d", len(items))
	}
}

// [正常系] Nameが空の場合TagNameがTitleとして使われること
func TestParseGitHubJSON_FallbackToTagName(t *testing.T) {
	releases := []GitHubRelease{
		{ID: 1, TagName: "v3.0.0-beta", Name: "", Body: "Beta release", HTMLURL: "https://example.com", PublishedAt: "2024-01-01T00:00:00Z"},
	}
	data, _ := json.Marshal(releases)

	items, err := ParseGitHubJSON(data, "org/repo")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if items[0].Title != "v3.0.0-beta" {
		t.Errorf("expected fallback title 'v3.0.0-beta', got '%s'", items[0].Title)
	}
}

// ============================================================
// RSS パースロジックテスト
// ============================================================

// [正常系] RSSフィードを正しくFetchedItemにマッピングできること
func TestParseRSSFeed_Normal(t *testing.T) {
	pubTime := time.Date(2024, 1, 20, 10, 0, 0, 0, time.UTC)
	feed := createMockFeed(
		"Test Blog",
		[]mockFeedItem{
			{GUID: "guid-001", Title: "New AI Model Released", Description: "OpenAI releases GPT-5", Link: "https://blog.example.com/gpt5", Published: &pubTime},
			{GUID: "guid-002", Title: "Research Update", Description: "Latest research findings", Link: "https://blog.example.com/research", Published: &pubTime},
		},
	)

	items := ParseRSSFeed(feed, "https://blog.example.com/feed")
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0].SourceType != "rss" {
		t.Errorf("expected SourceType 'rss', got '%s'", items[0].SourceType)
	}
	if items[0].SourceID != "guid-001" {
		t.Errorf("expected SourceID 'guid-001', got '%s'", items[0].SourceID)
	}
	if items[0].Title != "New AI Model Released" {
		t.Errorf("unexpected title: %s", items[0].Title)
	}
}

// [異常系] nilフィードでnilを返すこと
func TestParseRSSFeed_NilFeed(t *testing.T) {
	items := ParseRSSFeed(nil, "https://example.com/feed")
	if items != nil {
		t.Errorf("expected nil for nil feed, got %v", items)
	}
}

// [異常系] GUIDもLinkもないアイテムはスキップすること
func TestParseRSSFeed_NoIdentifier(t *testing.T) {
	feed := createMockFeed(
		"Test Blog",
		[]mockFeedItem{
			{GUID: "", Title: "No ID", Description: "Should be skipped", Link: ""},
			{GUID: "valid-guid", Title: "Valid Item", Description: "Should be included", Link: "https://example.com/valid"},
		},
	)

	items := ParseRSSFeed(feed, "https://example.com/feed")
	if len(items) != 1 {
		t.Fatalf("expected 1 item (no-id skipped), got %d", len(items))
	}
}

// [正常系] GUIDがない場合Linkをsource_idとして使用すること
func TestParseRSSFeed_FallbackToLink(t *testing.T) {
	feed := createMockFeed(
		"Test Blog",
		[]mockFeedItem{
			{GUID: "", Title: "Fallback Item", Description: "Uses link as ID", Link: "https://example.com/article-99"},
		},
	)

	items := ParseRSSFeed(feed, "https://example.com/feed")
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].SourceID != "https://example.com/article-99" {
		t.Errorf("expected SourceID fallback to link, got '%s'", items[0].SourceID)
	}
}

// ============================================================
// httptest を用いたフェッチャー統合テスト
// ============================================================

// [正常系] ArxivFetcherがhttptestサーバーから正しくデータ取得すること
func TestArxivFetcher_WithHTTPTest(t *testing.T) {
	mockXML := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/2401.12345v1</id>
    <title>Test Paper</title>
    <summary>A test summary for unit testing.</summary>
    <published>2024-01-20T00:00:00Z</published>
    <link href="http://arxiv.org/abs/2401.12345v1" rel="alternate" type="text/html"/>
  </entry>
</feed>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockXML))
	}))
	defer server.Close()

	fetcher := NewArxivFetcher(server.URL, []string{"cs.LG"}, server.Client())
	items, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Title != "Test Paper" {
		t.Errorf("expected title 'Test Paper', got '%s'", items[0].Title)
	}
}

// [正常系] GitHubFetcherがhttptestサーバーから正しくデータ取得すること
func TestGitHubFetcher_WithHTTPTest(t *testing.T) {
	releases := []GitHubRelease{
		{
			ID:          1001,
			TagName:     "v2.0.0",
			Name:        "Major Release 2.0",
			Body:        "Breaking changes and new API.",
			HTMLURL:     "https://github.com/test/repo/releases/tag/v2.0.0",
			PublishedAt: "2024-03-01T12:00:00Z",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if strings.Contains(r.URL.Path, "/releases") {
			json.NewEncoder(w).Encode(releases)
		} else {
			// Mock repository metadata response for getRepoStars
			w.Write([]byte(`{"stargazers_count": 2000, "created_at": "2024-01-01T00:00:00Z"}`))
		}
	}))
	defer server.Close()

	fetcher := NewGitHubFetcher(server.URL, []string{"test/repo"}, server.Client())
	fetcher.StarThreshold = 0 // Bypass star filter for testing fetch logic
	items, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Title != "Major Release 2.0" {
		t.Errorf("expected title 'Major Release 2.0', got '%s'", items[0].Title)
	}
}

func TestGitHubFetcher_EvaluateOwnerScore(t *testing.T) {
	fetcher := &GitHubFetcher{}

	tests := []struct {
		repoPath      string
		expectedScore int
		expectedOwner string
	}{
		{"google/go-cmp", 50, "google"},
		{"openai/gpt-3", 50, "openai"},
		{"hashicorp/terraform", 30, "hashicorp"},
		{"kubernetes/kubernetes", 30, "kubernetes"},
		{"vllm-project/vllm", 30, "vllm-project"},
		{"ollama/ollama", 30, "ollama"},
		{"ultralytics/ultralytics", 30, "ultralytics"},
		{"tiangolo/fastapi", 30, "tiangolo"},
		{"docker/compose", 30, "docker"},
		{"aws-samples/aws-cdk-examples", 10, "aws-samples"},
		{"google-research/bert", 10, "google-research"},
		{"owner/random-repo", 0, ""},
	}

	for _, tt := range tests {
		item := &models.FetchedItem{Metadata: make(map[string]string)}
		fetcher.evaluateOwnerScore(item, tt.repoPath)

		if item.Score != tt.expectedScore {
			t.Errorf("For %s, expected score %d, got %d", tt.repoPath, tt.expectedScore, item.Score)
		}
		if tt.expectedOwner != "" {
			if item.Metadata["renowned_owner"] != tt.expectedOwner {
				t.Errorf("For %s, expected renowned_owner %s, got %s", tt.repoPath, tt.expectedOwner, item.Metadata["renowned_owner"])
			}
		} else {
			if _, ok := item.Metadata["renowned_owner"]; ok {
				t.Errorf("For %s, expected no renowned_owner, got %s", tt.repoPath, item.Metadata["renowned_owner"])
			}
		}
	}
}

// [異常系] サーバーが500を返した場合エラーが返却されること
func TestArxivFetcher_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fetcher := NewArxivFetcher(server.URL, []string{"cs.LG"}, server.Client())
	_, err := fetcher.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// [異常系] コンテキストキャンセル時にフェッチが中断されること
func TestArxivFetcher_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // Simulate slow response
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	fetcher := NewArxivFetcher(server.URL, []string{"cs.LG"}, server.Client())
	_, err := fetcher.Fetch(ctx)
	if err == nil {
		t.Fatal("expected error due to context cancellation, got nil")
	}
}

// ============================================================
// ヘルパー: gofeed.Feed のモック生成
// ============================================================

type mockFeedItem struct {
	GUID        string
	Title       string
	Description string
	Link        string
	Published   *time.Time
}

func createMockFeed(title string, items []mockFeedItem) *gofeed.Feed {
	feed := &gofeed.Feed{
		Title: title,
	}
	for _, mi := range items {
		item := &gofeed.Item{
			GUID:        mi.GUID,
			Title:       mi.Title,
			Description: mi.Description,
			Link:        mi.Link,
		}
		if mi.Published != nil {
			item.PublishedParsed = mi.Published
		}
		feed.Items = append(feed.Items, item)
	}
	return feed
}

// [正常系] Batch APIを利用した会議情報と引用数の拡張スコアリング（およびバージョン番号の除去）が正しく動作すること
func TestArxivFetcher_EnhancedScoring(t *testing.T) {
	arxivXML := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:arxiv="http://arxiv.org/schemas/atom">
  <entry>
    <id>http://arxiv.org/abs/2403.00001v1</id>
    <title>Great AI Paper</title>
    <summary>Testing enhanced scoring with v1.</summary>
    <published>2024-03-20T00:00:00Z</published>
    <link href="http://arxiv.org/abs/2403.00001v1" rel="alternate" type="text/html"/>
    <author><name>Author</name></author>
    <arxiv:journal_ref>accepted at NEURIPS 2024</arxiv:journal_ref>
  </entry>
  <entry>
    <id>http://arxiv.org/abs/2403.00002v2</id>
    <title>Another Great Paper</title>
    <summary>Testing version stripping with v2.</summary>
    <published>2024-03-21T00:00:00Z</published>
    <link href="http://arxiv.org/abs/2403.00002v2" rel="alternate" type="text/html"/>
    <author><name>Author</name></author>
    <arxiv:comment>nips 2023 spotlight</arxiv:comment>
  </entry>
</feed>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/query") {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(arxivXML))
		} else if strings.Contains(r.URL.Path, "/graph/v1/paper/batch") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Mock Batch Response: Note that IDs here DO NOT have 'v1' or 'v2'
			w.Write([]byte(`[
				{
					"paperId": "id1",
					"externalIds": {"ArXiv": "2403.00001"},
					"citationCount": 100,
					"influentialCitationCount": 10
				},
				{
					"paperId": "id2",
					"externalIds": {"ArXiv": "2403.00002"},
					"citationCount": 200,
					"influentialCitationCount": 15
				}
			]`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	fetcher := NewArxivFetcher(server.URL, []string{"cs.AI"}, server.Client())
	fetcher.S2BaseURL = server.URL
	fetcher.S2MinScore = 0

	items, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Verify first item (2403.00001v1 matches 2403.00001: 40 + influence: 20 = 60)
	if items[0].Score < 60 {
		t.Errorf("expected score >= 60 for versioned item 1, got %d", items[0].Score)
	}

	// Verify second item (2403.00002v2 matches 2403.00002: 40 + influence: 20 = 60)
	if items[1].Score < 60 {
		t.Errorf("expected score >= 60 for versioned item 2, got %d", items[1].Score)
	}
}
