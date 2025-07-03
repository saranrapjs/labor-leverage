package edgar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// EdgarClient handles communications with Edgar APIs with rate limiting
type EdgarClient struct {
	userAgent  string
	httpClient *http.Client
}

// rateLimitedTransport wraps an HTTP transport with rate limiting
type rateLimitedTransport struct {
	transport http.RoundTripper
	limiter   *rate.Limiter
}

// RoundTrip implements the http.RoundTripper interface with rate limiting
func (r *rateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := r.limiter.Wait(req.Context()); err != nil {
		return nil, err
	}
	return r.transport.RoundTrip(req)
}

// NewEdgarClient creates a new Edgar API client with rate limiting
func NewEdgarClient(userAgent string, rateLimit int) *EdgarClient {
	if rateLimit <= 0 {
		rateLimit = 10 // Default to 10 requests per second
	}

	// Create rate-limited transport
	transport := &rateLimitedTransport{
		transport: http.DefaultTransport,
		limiter:   rate.NewLimiter(rate.Limit(rateLimit), rateLimit),
	}

	// Create HTTP client with rate-limited transport
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	return &EdgarClient{
		userAgent:  userAgent,
		httpClient: httpClient,
	}
}

// LoadSubmissions fetches and parses Edgar submissions data for a given CIK number
func (c *EdgarClient) LoadSubmissions(ctx context.Context, cik string) (*Submissions, error) {
	// Format CIK to 10 digits with leading zeros
	formattedCIK := fmt.Sprintf("%010s", cik)

	// Construct the API URL
	url := fmt.Sprintf("https://data.sec.gov/submissions/CIK%s.json", formattedCIK)
		fmt.Println(url)

	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent header
	req.Header.Set("User-Agent", c.userAgent)

	// Make HTTP request (rate limiting handled by transport)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch data from SEC API: %w", err)
	}
	defer resp.Body.Close()

	// Check if request was successful
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SEC API returned status %d", resp.StatusCode)
	}

	// Parse JSON response
	var submissions Submissions
	if err := json.NewDecoder(resp.Body).Decode(&submissions); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return &submissions, nil
}

// LoadDocument fetches a filing document using the Filing information
func (c *EdgarClient) LoadDocument(ctx context.Context, cik string, filing Filing) ([]byte, error) {
	// Remove hyphens from accession number for URL formatting
	accessionNumber := strings.ReplaceAll(filing.AccessionNumber, "-", "")
	
	// Construct the document URL
	url := fmt.Sprintf("https://www.sec.gov/Archives/edgar/data/%s/%s/%s", 
		cik, accessionNumber, filing.PrimaryDocument)

	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set User-Agent header
	req.Header.Set("User-Agent", c.userAgent)
	
	// Make HTTP request (rate limiting handled by transport)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch document from SEC: %w", err)
	}
	defer resp.Body.Close()
	
	// Check if request was successful
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SEC returned status %d for document request", resp.StatusCode)
	}
	
	// Read the document content
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read document content: %w", err)
	}
	
	return content, nil
}
