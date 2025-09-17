package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/saranrapjs/labor-leverage/pkg/db"
	"github.com/saranrapjs/labor-leverage/pkg/edgar"
	"github.com/saranrapjs/labor-leverage/pkg/facts"
	"github.com/saranrapjs/labor-leverage/pkg/irs"
	"github.com/saranrapjs/labor-leverage/pkg/irsform"
	"github.com/saranrapjs/labor-leverage/pkg/ixbrl"
	"golang.org/x/text/message"
)

//go:embed single.html
var templateHTML string

//go:embed index.html
var indexHTML string

//go:embed styles.css
var stylesCSS string

var printer = message.NewPrinter(message.MatchLanguage("en"))

// Template functions
var templateFuncs = template.FuncMap{
	"ratio": func(a,b float64) string {
		return fmt.Sprintf("%.0f", (a/b) * 100)
	},
	"divide": func(a,b interface{}) string {
		var aVal, bVal float64
		
		switch v := a.(type) {
		case float64:
			aVal = v
		case int:
			aVal = float64(v)
		default:
			return "N/A"
		}
		
		switch v := b.(type) {
		case float64:
			bVal = v
		case int:
			bVal = float64(v)
		default:
			return "N/A"
		}
		
		if bVal == 0 {
			return "N/A"
		}
		
		return fmt.Sprintf("%.0f", aVal/bVal)
	},
	"formatCurrency": func(val interface{}) string {
		switch v := val.(type) {
		case float64:
			return printer.Sprintf("$%.0f", v)
		case int:
			return printer.Sprintf("$%d", v)
		default:
			return fmt.Sprintf("$%v", v)
		}
	},
	"formatCount": func(val interface{}) string {
		switch v := val.(type) {
		case float64:
			return printer.Sprintf("%.0f", v)
		case int:
			return printer.Sprintf("%d", v)
		default:
			return fmt.Sprintf("%v", v)
		}
	},
	"formatNonFraction": func(nf *ixbrl.NonFraction) string {
		val := nf.ScaledNumber()
		return printer.Sprintf("$%.0f", val)
	},
	"formatNonFractionPerEmployee": func(nf *ixbrl.NonFraction, employeeCount int) template.HTML {
		val := nf.ScaledNumber()
		formatted := printer.Sprintf("$%.0f", val)
		
		if employeeCount > 0 {
			perEmployee := val / float64(employeeCount)
			perEmployeeFormatted := printer.Sprintf("$%.0f", perEmployee)
			return template.HTML(formatted + ` <span style="color: #666; font-size: 0.9em;">(` + perEmployeeFormatted + `/employee)</span>`)
		}
		
		return template.HTML(formatted)
	},
}

var tpl = template.Must(template.New("facts").Funcs(templateFuncs).Parse(templateHTML))
var indexTemplate = template.Must(template.New("index").Parse(indexHTML))

const cacheMaxAge = 30 * 24 * time.Hour // 1 month

// OrganizationItem represents a simplified organization with just title and path
type OrganizationItem struct {
	Title string `json:"title"` // Company/organization name
	Path  string `json:"path"`  // URL path to access the organization
}

type Server struct {
	db        *db.DB
	client    *edgar.EdgarClient
	irsClient *irs.IRSClient
}

func NewServer(database *db.DB) *Server {
	// Initialize Edgar client for network requests
	userAgent := "Jeff Sisson (jeff@bigboy.us)"
	client := edgar.NewEdgarClient(userAgent, 10)
	
	// Initialize IRS client for 2024 data
	irsClient, err := irs.NewIRSClient(os.Getenv("CACHE_DIR"), "2024")
	if err != nil {
		log.Fatalf("Failed to initialize IRS client: %v", err)
	}
	
	server := &Server{
		db:        database,
		client:    client,
		irsClient: irsClient,
	}
	
	// Populate search cache if empty
	if err := server.ensureSearchCachePopulated(); err != nil {
		log.Printf("Warning: Failed to populate search cache: %v", err)
	}
	
	return server
}

// ensureSearchCachePopulated populates the search cache if it's empty
func (s *Server) ensureSearchCachePopulated() error {
	count, err := s.db.GetSearchCacheCount()
	if err != nil {
		return fmt.Errorf("failed to get search cache count: %w", err)
	}
	
	if count > 0 {
		log.Printf("Search cache already populated with %d items", count)
		return nil
	}
	
	log.Println("Populating search cache...")
	
	// Clear cache first
	if err := s.db.ClearSearchCache(); err != nil {
		return fmt.Errorf("failed to clear search cache: %w", err)
	}
	
	// Prepare batch items
	var items []db.SearchCacheItem
	
	// Add Edgar data
	for _, ticker := range edgar.TickersData {
		items = append(items, db.SearchCacheItem{
			Title:      ticker.Title,
			Path:       fmt.Sprintf("/ticker/%s", ticker.Ticker),
			SourceType: "SEC",
		})
	}
	
	// Add IRS data
	eins := map[string]bool{}
	for _, nonprofit := range s.irsClient.NonProfits {
		if _, seen := eins[nonprofit.EIN]; seen {
			continue
		}
		eins[nonprofit.EIN] = true
		items = append(items, db.SearchCacheItem{
			Title:      nonprofit.Name,
			Path:       fmt.Sprintf("/ein/%s", nonprofit.EIN),
			SourceType: "IRS",
		})
	}
	
	// Store items in batches
	if err := s.db.StoreSearchCacheItems(items); err != nil {
		return fmt.Errorf("failed to store search cache items: %w", err)
	}
	
	finalCount, err := s.db.GetSearchCacheCount()
	if err != nil {
		return fmt.Errorf("failed to get final search cache count: %w", err)
	}
	
	log.Printf("Search cache populated with %d items", finalCount)
	return nil
}

// handleFilings handles GET /api/ticker/{ticker}
func (s *Server) handleTicker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ticker := strings.ToUpper(r.PathValue("ticker"))
	if ticker == "" {
		http.Error(w, "Ticker parameter is required", http.StatusBadRequest)
		return
	}
	// Convert ticker to CIK
	cik, err := edgar.Ticker2CIK(ticker)
	if err != nil {
		http.Error(w, fmt.Sprintf("Ticker %s not found: %v", ticker, err), http.StatusNotFound)
		return
	}
	r.SetPathValue("cik", cik)
	s.handleFilings(w, r)
}
func (s *Server) handleCik(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cik := strings.ToUpper(r.PathValue("cik"))
	if cik == "" {
		http.Error(w, "Ticker parameter is required", http.StatusBadRequest)
		return
	}
	// Convert ticker to CIK
	ticker, err := edgar.CIK2Ticker(cik)
	if err != nil {
		http.Error(w, fmt.Sprintf("cik %s not found: %v", cik, err), http.StatusNotFound)
		return
	}
	r.SetPathValue("ticker", ticker)
	s.handleFilings(w, r)
}

// handleIndex serves the root index page with ticker search
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	w.Header().Set("Content-Type", "text/html")
	err := indexTemplate.Execute(w, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error rendering template: %v", err), http.StatusInternalServerError)
		return
	}
}

// handleOrganizationsJSON handles GET /api/organizations.json to return organization data as JSON
func (s *Server) handleOrganizationsJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var organizations []OrganizationItem
	
	// Add Edgar data
	for _, ticker := range edgar.TickersData {
		organizations = append(organizations, OrganizationItem{
			Title: ticker.Title,
			Path:  fmt.Sprintf("/ticker/%s", ticker.Ticker),
		})
	}
	
	eins := map[string]bool{}

	// Add IRS data
	for _, nonprofit := range s.irsClient.NonProfits {
		if _, seen := eins[nonprofit.EIN]; seen {
			continue
		}
		eins[nonprofit.EIN] = true
		organizations = append(organizations, OrganizationItem{
			Title: nonprofit.Name,
			Path:  fmt.Sprintf("/ein/%s", nonprofit.EIN),
		})
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour
	if err := json.NewEncoder(w).Encode(organizations); err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}
}

// handleSearchAPI handles GET /api/search?q={query}&limit={limit} to return filtered organization data
func (s *Server) handleSearchAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]OrganizationItem{})
		return
	}
	
	limitStr := r.URL.Query().Get("limit")
	limit := 10 // default limit
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 50 {
			limit = parsedLimit
		}
	}
	
	// Search using cached data with FTS
	results, err := s.db.SearchCache(query, limit)
	if err != nil {
		log.Printf("Search cache failed: %v", err)
		http.Error(w, "Search failed", http.StatusInternalServerError)
		return
	}
	
	// Convert to OrganizationItem format
	var organizations []OrganizationItem
	for _, result := range results {
		organizations = append(organizations, OrganizationItem{
			Title: result.Title,
			Path:  result.Path,
		})
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300") // Cache for 5 minutes
	if err := json.NewEncoder(w).Encode(organizations); err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}
}


func (s *Server) handleFilings(w http.ResponseWriter, r *http.Request) {
	ticker := strings.ToUpper(r.PathValue("ticker"))
	cik := strings.ToUpper(r.PathValue("cik"))
	w.Header().Set("x-ticker", ticker)
	w.Header().Set("x-cik", cik)

	var factData *facts.Facts
	var err error

	// Check if facts exist in database and if they're fresh
	stale, err := s.db.AreFactsStale(cik, cacheMaxAge)
	if err != nil {
		log.Printf("Error checking facts staleness for CIK %s: %v", cik, err)
		stale = true // Assume stale on error
	}

	if !stale {
		// Get facts from database (they're fresh)
		factData, err = s.db.GetFacts(cik)
		if err != nil {
			log.Printf("Error retrieving facts from database for CIK %s: %v", cik, err)
			stale = true // Force network fetch on database error
		}
	}

	if stale {
		// Facts are stale or don't exist, fetch from network
		log.Printf("Facts for CIK %s are stale or missing, fetching from network", cik)
		factData, err = s.downloadAndProcessFacts(r.Context(), cik, ticker)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to process facts: %v", err), http.StatusInternalServerError)
			return
		}

		// Store the freshly fetched facts in database
		if err := s.db.StoreFacts(factData); err != nil {
			log.Printf("Warning: Failed to store facts in database for CIK %s: %v", cik, err)
			// Continue serving even if storage fails
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tpl.Execute(w, factData); err != nil {
		log.Printf("Failed to execute template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// downloadAndProcessFacts downloads and processes Edgar data from the network
func (s *Server) downloadAndProcessFacts(ctx context.Context, cik, ticker string) (*facts.Facts, error) {
	log.Printf("Downloading submissions for CIK %s...", cik)
	
	// Load submissions
	submissions, err := s.client.LoadSubmissions(ctx, cik)
	if err != nil {
		return nil, fmt.Errorf("failed to load submissions: %w", err)
	}

	// Search for filings
	filingTypes := []string{"10-K", "10-Q", "DEF 14A"}
	var foundFilings []edgar.Filing
	for _, filingType := range filingTypes {
		filing, found := submissions.Filings.Search(cik, filingType)
		if found {
			foundFilings = append(foundFilings, filing)
			log.Printf("Found %s filing: %s", filingType, filing.AccessionNumber)
		}
	}

	if len(foundFilings) == 0 {
		return nil, fmt.Errorf("no relevant filings found for CIK %s", cik)
	}

	// Download documents
	var filingDocs []edgar.Document
	for _, filing := range foundFilings {
		log.Printf("Downloading document for %s filing...", filing.Form)
		content, err := s.client.LoadDocument(ctx, cik, filing)
		if err != nil {
			log.Printf("Failed to download %s document: %v", filing.Form, err)
			continue
		}
		
		doc := edgar.Document{
			Filing:       filing,
			DocumentFile: content,
		}
		filingDocs = append(filingDocs, doc)
	}

	if len(filingDocs) == 0 {
		return nil, fmt.Errorf("failed to download any documents for CIK %s", cik)
	}

	// Get company name
	companyName, err := edgar.Ticker2CompanyName(ticker)
	if err != nil {
		log.Printf("Warning: Could not get company name for ticker %s: %v", ticker, err)
		companyName = "" // Use empty string if not found
	}

	// Extract facts
	return facts.ExtractFacts(cik, ticker, companyName, filingDocs)
}

// handleHealth handles GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// handleStyles serves the shared CSS file
func (s *Server) handleStyles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css")
	w.Write([]byte(stylesCSS))
}

// handleIRSCompany handles GET /irs/{ein} to fetch company XML data from IRS with caching
func (s *Server) handleIRSCompany(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	ein := r.PathValue("ein")
	if ein == "" {
		http.Error(w, "EIN parameter is required", http.StatusBadRequest)
		return
	}
	
	log.Printf("Fetching IRS data for EIN: %s", ein)
	
	var xmlData []byte
	var err error
	
	// Check if we have cached XML data
	staleXML, err := s.db.AreIRSReturnsStale(ein, cacheMaxAge)
	if err != nil {
		log.Printf("Error checking IRS return staleness for EIN %s: %v", ein, err)
		staleXML = true // Assume stale on error
	}
	
	if !staleXML {
		// Get XML from database (it's fresh)
		xmlData, err = s.db.GetIRSReturn(ein)
		if err != nil {
			log.Printf("Error retrieving IRS return from database for EIN %s: %v", ein, err)
			staleXML = true // Force network fetch on database error
		}
	}
	
	if staleXML {
		// Find the nonprofit to get the return type
		var returnType string
		for _, np := range s.irsClient.NonProfits {
			if strings.EqualFold(np.EIN, ein) && irsform.IsSupportedReturnType(np.ReturnType) {
				returnType = np.ReturnType
				break
			}
		}
		
		if returnType == "" {
			http.Error(w, fmt.Sprintf("EIN %s not found or unsupported return type", ein), http.StatusNotFound)
			return
		}
		
		// XML data is stale or doesn't exist, fetch from network
		log.Printf("IRS return for EIN %s is stale or missing, fetching from network", ein)
		xmlData, err = s.irsClient.FetchCompany(ein)
		if err != nil {
			log.Printf("Failed to fetch company data for EIN %s: %v", ein, err)
			http.Error(w, fmt.Sprintf("Failed to fetch company data: %v", err), http.StatusInternalServerError)
			return
		}
		
		// Store the XML data in database
		if err := s.db.StoreIRSReturn(ein, returnType, "2024", xmlData); err != nil {
			log.Printf("Warning: Failed to store IRS return in database for EIN %s: %v", ein, err)
			// Continue serving even if storage fails
		}
	}
	
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.Write(xmlData)
}

// handleIRSFacts handles GET /ein/{ein} to extract Facts from IRS return data with lazy loading
func (s *Server) handleIRSFacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	ein := r.PathValue("ein")
	if ein == "" {
		http.Error(w, "EIN parameter is required", http.StatusBadRequest)
		return
	}
	
	w.Header().Set("x-ein", ein)

	var factData *facts.Facts
	var err error

	// Check if facts exist in database and if they're fresh
	stale, err := s.db.AreFactsStale(ein, cacheMaxAge)
	if err != nil {
		log.Printf("Error checking facts staleness for EIN %s: %v", ein, err)
		stale = true // Assume stale on error
	}

	if !stale {
		// Get facts from database (they're fresh)
		factData, err = s.db.GetFacts(ein)
		if err != nil {
			log.Printf("Error retrieving facts from database for EIN %s: %v", ein, err)
			stale = true // Force network fetch on database error
		}
	}

	if stale {
		// Facts are stale or don't exist, fetch from network
		log.Printf("Facts for EIN %s are stale or missing, fetching from network", ein)
		factData, err = s.downloadAndProcessIRSFacts(r.Context(), ein)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to process facts: %v", err), http.StatusInternalServerError)
			return
		}

		// Store the freshly fetched facts in database
		if err := s.db.StoreFacts(factData); err != nil {
			log.Printf("Warning: Failed to store facts in database for EIN %s: %v", ein, err)
			// Continue serving even if storage fails
		}
	}

	// Return facts as HTML using the same template as ticker endpoint
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tpl.Execute(w, factData); err != nil {
		log.Printf("Failed to execute template for EIN %s: %v", ein, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// downloadAndProcessIRSFacts downloads and processes IRS data from the network
func (s *Server) downloadAndProcessIRSFacts(ctx context.Context, ein string) (*facts.Facts, error) {
	log.Printf("Downloading IRS data for EIN %s...", ein)
	
	var xmlData []byte
	var err error
	
	// Check if we have cached XML data
	staleXML, err := s.db.AreIRSReturnsStale(ein, cacheMaxAge)
	if err != nil {
		log.Printf("Error checking IRS return staleness for EIN %s: %v", ein, err)
		staleXML = true // Assume stale on error
	}
	
	if !staleXML {
		// Get XML from database (it's fresh)
		xmlData, err = s.db.GetIRSReturn(ein)
		if err != nil {
			log.Printf("Error retrieving IRS return from database for EIN %s: %v", ein, err)
			staleXML = true // Force network fetch on database error
		}
	}
	
	if staleXML {
		// XML data is stale or doesn't exist, fetch from network
		log.Printf("IRS return for EIN %s is stale or missing, fetching from network", ein)
		xmlData, err = s.irsClient.FetchCompany(ein)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch company data: %w", err)
		}
		
		// Find the nonprofit to get the return type and tax year
		var returnType, taxYear string
		for _, np := range s.irsClient.NonProfits {
			if strings.EqualFold(np.EIN, ein) {
				returnType = np.ReturnType
				taxYear = "2024" // Current year from IRS client
				break
			}
		}
		
		// Store the XML data in database
		if returnType != "" {
			if err := s.db.StoreIRSReturn(ein, returnType, taxYear, xmlData); err != nil {
				log.Printf("Warning: Failed to store IRS return in database for EIN %s: %v", ein, err)
				// Continue processing even if storage fails
			}
		}
	}
	
	// Parse the XML data
	reader := strings.NewReader(string(xmlData))
	returnData, err := irsform.Parse(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse XML data: %w", err)
	}
	
	// Extract facts using FromIRS (now handles all supported return types)
	factData, err := facts.FromIRS(returnData)
	if err != nil {
		return nil, fmt.Errorf("failed to extract facts from IRS data: %w", err)
	}
	
	// Set the EIN in the facts data
	factData.EIN = ein
	
	return factData, nil
}

func main() {
	// Initialize database
	database, err := db.New("edgar.db")
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Create server
	server := NewServer(database)

	// Set up routes using new pattern syntax
	mux := http.NewServeMux()
	mux.HandleFunc("GET /cik/{cik}", server.handleCik)
	mux.HandleFunc("GET /ticker/{ticker}", server.handleTicker)
	mux.HandleFunc("GET /irs/{ein}", server.handleIRSCompany)
	mux.HandleFunc("GET /ein/{ein}", server.handleIRSFacts)
	mux.HandleFunc("GET /api/organizations.json", server.handleOrganizationsJSON)
	mux.HandleFunc("GET /api/search", server.handleSearchAPI)
	mux.HandleFunc("GET /health", server.handleHealth)
	mux.HandleFunc("GET /styles.css", server.handleStyles)
	mux.HandleFunc("GET /", server.handleIndex)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Starting Edgar API server on port %s", port)
	if err := http.ListenAndServe(":" + port, mux); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
