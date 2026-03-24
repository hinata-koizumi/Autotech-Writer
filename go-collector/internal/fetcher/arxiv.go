package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/autotech-writer/go-collector/internal/common"
	"github.com/autotech-writer/go-collector/internal/models"
	"github.com/autotech-writer/go-collector/internal/sanitizer"
)

// ArxivFeed represents the Atom feed structure from arXiv API.
type ArxivFeed struct {
	XMLName xml.Name     `xml:"feed"`
	Entries []ArxivEntry `xml:"entry"`
}

// ArxivEntry represents a single entry in the arXiv Atom feed.
type ArxivEntry struct {
	ID         string        `xml:"id"`
	Title      string        `xml:"title"`
	Summary    string        `xml:"summary"`
	Published  string        `xml:"published"`
	Links      []ArxivLink   `xml:"link"`
	Authors    []ArxivAuthor `xml:"author"`
	JournalRef string        `xml:"http://arxiv.org/schemas/atom journal_ref"`
	Comment    string        `xml:"http://arxiv.org/schemas/atom comment"`
}

// ArxivLink represents a link element in the arXiv feed.
type ArxivLink struct {
	Href string `xml:"href,attr"`
	Type string `xml:"type,attr"`
	Rel  string `xml:"rel,attr"`
}

// ArxivAuthor represents an author element.
type ArxivAuthor struct {
	Name string `xml:"name"`
}

// ArxivFetcher fetches papers from the arXiv API.
type ArxivFetcher struct {
	BaseURL     string
	S2BaseURL   string
	Categories  []string
	MaxResults  int
	HTTPClient  *http.Client
	S2BatchSize int
	S2MinScore  int
	S2APIKey    string
}

// NewArxivFetcher creates a new ArxivFetcher.
func NewArxivFetcher(baseURL string, categories []string, client *http.Client) *ArxivFetcher {
	if baseURL == "" {
		baseURL = "https://export.arxiv.org"
	}
	if client == nil {
		client = &http.Client{Timeout: DefaultHTTPTimeout}
	}
	return &ArxivFetcher{
		BaseURL:     baseURL,
		S2BaseURL:   "https://api.semanticscholar.org",
		Categories:  categories,
		MaxResults:  DefaultArxivMaxResults,
		HTTPClient:  client,
		S2BatchSize: DefaultS2BatchSize,
		S2MinScore:  DefaultS2MinScore,
		S2APIKey:    os.Getenv("SEMANTIC_SCHOLAR_API_KEY"),
	}
}

// Fetch retrieves the latest papers from arXiv.
func (f *ArxivFetcher) Fetch(ctx context.Context) ([]models.FetchedItem, error) {
	urls := make([]string, len(f.Categories))
	for i, cat := range f.Categories {
		urls[i] = fmt.Sprintf("%s/api/query?search_query=cat:%s&sortBy=submittedDate&sortOrder=descending&max_results=%d", f.BaseURL, cat, f.MaxResults)
	}

	items, err := collectFromSources(ctx, f.HTTPClient, f.Categories, urls, func(data []byte, _ string) ([]models.FetchedItem, error) {
		return ParseArxivXML(data)
	})
	if err != nil {
		return items, err
	}

	for i := range items {
		if items[i].Metadata == nil {
			items[i].Metadata = make(map[string]string)
		}

		// 1. Provisional Scoring (Title/Summary)
		f.evaluateInstitutionScore(&items[i])

		// 2. Metadata-based Scoring (Conferences)
		f.evaluateConferenceScore(&items[i])
	}

	// 3. Batch Citation-based Scoring (Semantic Scholar)
	var targetIDs []string
	var targetItems []*models.FetchedItem

	for i := range items {
		// Only fetch citations for papers with a decent provisional score
		if items[i].Score >= targetS2MinScore(f.S2MinScore) {
			// S2 requires "arXiv:" prefix for arXiv IDs
			// Strip version number (e.g. 2403.00001v1 -> 2403.00001) for better matching
			id := items[i].SourceID
			if parts := strings.Split(id, "/"); len(parts) > 0 {
				id = parts[len(parts)-1]
			}
			id = strings.Split(id, "v")[0]

			targetIDs = append(targetIDs, "arXiv:"+id)
			targetItems = append(targetItems, &items[i])
		}
	}

	for i := 0; i < len(targetIDs); i += f.S2BatchSize {
		end := i + f.S2BatchSize
		if end > len(targetIDs) {
			end = len(targetIDs)
		}
		batchIDs := targetIDs[i:end]
		batchRefs := targetItems[i:end]

		citationMap, err := f.fetchCitationDataBatch(ctx, batchIDs)
		if err != nil {
			slog.Warn("Failed to fetch citation batch", "error", err)
			continue
		}

		for _, item := range batchRefs {
			id := item.SourceID
			if parts := strings.Split(id, "/"); len(parts) > 0 {
				id = parts[len(parts)-1]
			}
			id = strings.Split(id, "v")[0]

			if data, ok := citationMap["arXiv:"+id]; ok {
				if data.InfluentialCitationCount > 5 {
					item.Score += 20
					item.Metadata["citations"] = fmt.Sprintf("%d total, %d influential", data.CitationCount, data.InfluentialCitationCount)
				} else if data.CitationCount > 50 {
					item.Score += 10
					item.Metadata["citations"] = fmt.Sprintf("%d total", data.CitationCount)
				}
			}
		}
	}

	for i := range items {
		// Ensure we have a PDF URL as fallback
		shortID := items[i].SourceID
		if strings.Contains(shortID, "arxiv.org/abs/") {
			parts := strings.Split(shortID, "/")
			shortID = parts[len(parts)-1]
		}
		items[i].Metadata["pdf_url"] = fmt.Sprintf("%s/pdf/%s.pdf", f.BaseURL, shortID)

		// 4. Full Content Processing (LaTeX)
		tex, texErr := sanitizer.ProcessArxivLatex(ctx, f.HTTPClient, f.BaseURL, items[i].SourceID, fetchURL)
		if texErr != nil {
			slog.Debug("Failed to fetch/process LaTeX", "id", items[i].SourceID, "error", texErr)
		} else if tex != "" {
			items[i].FullContent = tex

			// 3. Refined Scoring (LaTeX Source)
			f.refineScoreFromContent(&items[i])
		}

		// Final Breaking News Flag
		if items[i].Score >= common.BreakingNewsScoreThreshold {
			items[i].IsBreakingNews = true
		}
	}
	return items, nil
}

func (f *ArxivFetcher) refineScoreFromContent(item *models.FetchedItem) {
	if item.FullContent == "" {
		return
	}
	// Re-evaluate score using full content (LaTeX might contain more explicit affiliations)
	content := strings.ToLower(item.FullContent)

	// If current score is already high, we just want to confirm or find a better match
	// If current score is low, we check if LaTeX source reveals a Tier 1/2 affiliation
	if match := findTierMatch(content, common.ArxivTier1); match != "" {
		if item.Score < 50 {
			item.Score = 50
			item.Metadata["institution"] = match + " (refined)"
		}
		return
	}
	if match := findTierMatch(content, common.ArxivTier2); match != "" {
		if item.Score < 30 {
			item.Score = 30
			item.Metadata["institution"] = match + " (refined)"
		}
		return
	}
}

func (f *ArxivFetcher) Name() string {
	return "arXiv"
}

// evaluateInstitutionScore assigns a priority score based on the authors' affiliations.
func (f *ArxivFetcher) evaluateInstitutionScore(item *models.FetchedItem) {
	content := strings.ToLower(item.Title + " " + item.Summary)

	if match := findTierMatch(content, common.ArxivTier1); match != "" {
		item.Score = 50
		item.Metadata["institution"] = match
		return
	}
	if match := findTierMatch(content, common.ArxivTier2); match != "" {
		item.Score = 30
		item.Metadata["institution"] = match
		return
	}
	if match := findTierMatch(content, common.ArxivTier3); match != "" {
		item.Score = 10
		item.Metadata["institution"] = match
		return
	}
}

func (f *ArxivFetcher) evaluateConferenceScore(item *models.FetchedItem) {
	content := strings.ToLower(item.Metadata["journal_ref"] + " " + item.Metadata["comment"])
	for conf, pattern := range common.TopConferencePatterns {
		matched, _ := regexp.MatchString(pattern, content)
		if matched {
			item.Score += 40
			item.Metadata["conference"] = conf
			return
		}
	}
}

type S2BatchResponse []struct {
	PaperId     string `json:"paperId"`
	ExternalIds struct {
		ArXiv string `json:"ArXiv"`
	} `json:"externalIds"`
	CitationCount            int `json:"citationCount"`
	InfluentialCitationCount int `json:"influentialCitationCount"`
}

func (f *ArxivFetcher) fetchCitationDataBatch(ctx context.Context, ids []string) (map[string]struct {
	CitationCount            int
	InfluentialCitationCount int
}, error) {
	resultMap := make(map[string]struct {
		CitationCount            int
		InfluentialCitationCount int
	})

	url := fmt.Sprintf("%s/graph/v1/paper/batch?fields=externalIds,citationCount,influentialCitationCount", f.S2BaseURL)

	body, err := json.Marshal(map[string][]string{"ids": ids})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if f.S2APIKey != "" {
		req.Header.Set("x-api-key", f.S2APIKey)
	}

	resp, err := f.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("S2 Batch API returned status %d", resp.StatusCode)
	}

	var batchResp S2BatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		return nil, err
	}

	for _, data := range batchResp {
		if data.ExternalIds.ArXiv != "" {
			resultMap["arXiv:"+data.ExternalIds.ArXiv] = struct {
				CitationCount            int
				InfluentialCitationCount int
			}{
				CitationCount:            data.CitationCount,
				InfluentialCitationCount: data.InfluentialCitationCount,
			}
		}
	}
	return resultMap, nil
}

func targetS2MinScore(val int) int {
	if val <= 0 {
		return DefaultS2MinScore
	}
	return val
}

func findTierMatch(content string, tier []string) string {
	for _, k := range tier {
		if strings.Contains(content, k) {
			return k
		}
	}
	return ""
}

// ParseArxivXML parses arXiv Atom XML into FetchedItems.
func ParseArxivXML(data []byte) ([]models.FetchedItem, error) {
	var feed ArxivFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("unmarshaling arXiv XML: %w", err)
	}

	var items []models.FetchedItem
	for _, entry := range feed.Entries {
		if entry.ID == "" {
			continue
		}

		publishedAt, _ := time.Parse(time.RFC3339, entry.Published)
		entryURL := getArxivLink(entry.Links)

		rawData, _ := json.Marshal(entry)

		metadata := make(map[string]string)
		if entry.JournalRef != "" {
			metadata["journal_ref"] = entry.JournalRef
		}
		if entry.Comment != "" {
			metadata["comment"] = entry.Comment
		}

		items = append(items, models.FetchedItem{
			SourceType:  "arxiv",
			SourceID:    entry.ID,
			Title:       entry.Title,
			Summary:     entry.Summary,
			URL:         entryURL,
			PublishedAt: publishedAt,
			Metadata:    metadata,
			RawData:     string(rawData),
		})
	}
	return items, nil
}

func getArxivLink(links []ArxivLink) string {
	for _, link := range links {
		if link.Rel == "alternate" || link.Type == "text/html" {
			return link.Href
		}
	}
	if len(links) > 0 {
		return links[0].Href
	}
	return ""
}
