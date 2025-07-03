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
	"strings"
	"time"

	"github.com/saranrapjs/labor-leverage/pkg/db"
	"github.com/saranrapjs/labor-leverage/pkg/edgar"
	"github.com/saranrapjs/labor-leverage/pkg/facts"
	"github.com/saranrapjs/labor-leverage/pkg/ixbrl"
	"golang.org/x/text/message"
)

//go:embed template.html
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
	"divide": func(a,b float64) string {
		return fmt.Sprintf("%.0f", a/b)
	},
	"formatCurrency": func(val float64) string {
		return printer.Sprintf("$%.0f", val)
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

type Server struct {
	db     *db.DB
	client *edgar.EdgarClient
}

func NewServer(database *db.DB) *Server {
	// Initialize Edgar client for network requests
	userAgent := "Jeff Sisson (jeff@bigboy.us)"
	client := edgar.NewEdgarClient(userAgent, 10)
	
	return &Server{
		db:     database,
		client: client,
	}
}



func (s *Server) handleAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get CIKs with facts from database
	ciks, err := s.db.ListFactsCIKs()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get CIKs: %v", err), http.StatusInternalServerError)
		return
	}
	for _, cik := range ciks {
		w.Write([]byte(cik + "\n"))
	}
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
	err := indexTemplate.Execute(w, edgar.TickersData)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error rendering template: %v", err), http.StatusInternalServerError)
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
	mux.HandleFunc("GET /all", server.handleAll)
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
