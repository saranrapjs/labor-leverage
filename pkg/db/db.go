package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
	"github.com/saranrapjs/labor-leverage/pkg/edgar"
	"github.com/saranrapjs/labor-leverage/pkg/facts"
)

// DB wraps a SQLite database connection for Edgar data storage
type DB struct {
	conn *sql.DB
}

// New creates a new database connection and initializes tables
func New(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.createTables(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// createTables creates the required tables if they don't exist
func (db *DB) createTables() error {
	// Create submissions table
	submissionsSQL := `
		CREATE TABLE IF NOT EXISTS submissions (
			cik TEXT PRIMARY KEY,
			data BLOB NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`
	if _, err := db.conn.Exec(submissionsSQL); err != nil {
		return fmt.Errorf("failed to create submissions table: %w", err)
	}

	// Create filings table
	filingsSQL := `
		CREATE TABLE IF NOT EXISTS filings (
			accession_number TEXT PRIMARY KEY,
			cik TEXT NOT NULL,
			form_name TEXT NOT NULL,
			filing_date TEXT NOT NULL,
			filing BLOB NOT NULL,
			primary_document BLOB NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(cik, form_name)
		);
	`
	if _, err := db.conn.Exec(filingsSQL); err != nil {
		return fmt.Errorf("failed to create filings table: %w", err)
	}

	// Create facts table with generic ID support
	factsSQL := `
		CREATE TABLE IF NOT EXISTS facts (
			id TEXT PRIMARY KEY,
			source_type TEXT NOT NULL,
			data BLOB NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			company_name TEXT DEFAULT ''
		);
	`
	if _, err := db.conn.Exec(factsSQL); err != nil {
		return fmt.Errorf("failed to create facts table: %w", err)
	}

	// Create index on source_type for efficient queries
	indexSQL := `CREATE INDEX IF NOT EXISTS idx_facts_source_type ON facts(source_type);`
	if _, err := db.conn.Exec(indexSQL); err != nil {
		return fmt.Errorf("failed to create source_type index: %w", err)
	}

	// Create IRS returns table
	irsReturnsSQL := `
		CREATE TABLE IF NOT EXISTS irs_returns (
			ein TEXT PRIMARY KEY,
			return_type TEXT NOT NULL,
			tax_year TEXT NOT NULL,
			xml_data BLOB NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`
	if _, err := db.conn.Exec(irsReturnsSQL); err != nil {
		return fmt.Errorf("failed to create irs_returns table: %w", err)
	}

	// Create search cache table using FTS for efficient searching
	searchCacheSQL := `
		CREATE VIRTUAL TABLE IF NOT EXISTS search_cache USING fts5(
			title,
			path,
			source_type,
			created_at UNINDEXED,
			updated_at UNINDEXED
		);
	`
	if _, err := db.conn.Exec(searchCacheSQL); err != nil {
		return fmt.Errorf("failed to create search_cache table: %w", err)
	}

	return nil
}

// StoreSubmissions stores the submissions JSON data in the database
func (db *DB) StoreSubmissions(cik string, submissions *edgar.Submissions) error {
	// Marshal submissions to JSON
	data, err := json.Marshal(submissions)
	if err != nil {
		return fmt.Errorf("failed to marshal submissions: %w", err)
	}

	// Insert or replace the submissions data
	query := `
		INSERT OR REPLACE INTO submissions (cik, data) 
		VALUES (?, ?)
	`
	_, err = db.conn.Exec(query, cik, data)
	if err != nil {
		return fmt.Errorf("failed to store submissions: %w", err)
	}

	return nil
}

// GetSubmissions retrieves submissions data from the database
func (db *DB) GetSubmissions(cik string) (*edgar.Submissions, error) {
	query := "SELECT data FROM submissions WHERE cik = ?"
	
	var data []byte
	err := db.conn.QueryRow(query, cik).Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("submissions not found for CIK %s", cik)
		}
		return nil, fmt.Errorf("failed to query submissions: %w", err)
	}

	var submissions edgar.Submissions
	if err := json.Unmarshal(data, &submissions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal submissions: %w", err)
	}

	return &submissions, nil
}

// StoreFiling stores a filing document in the database
func (db *DB) StoreFiling(cik string, filing edgar.Filing, data []byte) error {
	query := `
		INSERT OR REPLACE INTO filings (cik, form_name, accession_number, filing_date, filing, primary_document) 
		VALUES (?, ?, ?, ?, ?, ?)
	`
	filingJson, err := json.Marshal(filing)
	if err != nil {
		return fmt.Errorf("failed to serialize filing: %w", err)
	}
	if _, err := db.conn.Exec(query, cik, filing.Form, filing.AccessionNumber, filing.FilingDate, filingJson, data); err != nil {
		return fmt.Errorf("failed to store filing: %w", err)
	}

	return nil
}

// GetFiling retrieves a filing document from the database and returns the Filing info and document data
func (db *DB) GetFiling(cik, formName string) (*edgar.Filing, []byte, error) {
	query := `
		SELECT filing, data 
		FROM filings 
		WHERE cik = ? AND form_name = ?
	`
	
	var filing edgar.Filing
	var filingJson, document []byte
	err := db.conn.QueryRow(query, cik, formName).Scan(
		&filingJson, &document)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("filing not found for CIK %s, form %s", cik, formName)
		}
		return nil, nil, fmt.Errorf("failed to query filing: %w", err)
	}

	if err := json.Unmarshal(filingJson, &filing); err != nil {
		return nil, nil, fmt.Errorf("failed to query filing: %w", err)		
	}

	return &filing, document, nil
}

func (db *DB) ListAll() ([]string, error) {
	query := `
		SELECT cik
		FROM filings 
		GROUP BY cik
	`
	
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query filings: %w", err)
	}
	defer rows.Close()

	var ciks []string
	for rows.Next() {
		var cik string
		if err := rows.Scan(&cik); err != nil {
			return nil, fmt.Errorf("failed to scan filing row: %w", err)
		}
		ciks = append(ciks, cik)
	}
	return ciks, nil
}

// ListFilings returns all filing metadata for a given CIK as edgar.Filing structs
func (db *DB) ListFilings(cik string) ([]edgar.Document, error) {
	query := `
		SELECT filing, primary_document
		FROM filings 
		WHERE cik = ? 
		ORDER BY filing_date DESC
	`
	
	rows, err := db.conn.Query(query, cik)
	if err != nil {
		return nil, fmt.Errorf("failed to query filings: %w", err)
	}
	defer rows.Close()

	var filings []edgar.Document
	for rows.Next() {
		var filingJson, document []byte
		var filing edgar.Document
		if err := rows.Scan(&filingJson, &document); err != nil {
			return nil, fmt.Errorf("failed to scan filing row: %w", err)
		}
		if err := json.Unmarshal(filingJson, &filing); err != nil {
			return nil, fmt.Errorf("failed to query filing: %w", err)		
		}
		filing.CIK = cik
		filing.DocumentFile = document
		filings = append(filings, filing)
	}

	return filings, nil
}

// StoreFacts stores Facts data in the database
func (db *DB) StoreFacts(f *facts.Facts) error {
	// Marshal facts to JSON
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("failed to marshal facts: %w", err)
	}

	// Determine ID and source type
	var id, sourceType string
	if f.CIK != "" {
		id = f.CIK
		sourceType = "SEC"
	} else if f.EIN != "" {
		id = f.EIN
		sourceType = "IRS"
	} else {
		return fmt.Errorf("facts must have either CIK or EIN")
	}

	// Insert or replace the facts data
	query := `
		INSERT OR REPLACE INTO facts (id, source_type, company_name, data, updated_at) 
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	`
	_, err = db.conn.Exec(query, id, sourceType, f.CompanyName, data)
	if err != nil {
		return fmt.Errorf("failed to store facts: %w", err)
	}

	return nil
}

// GetFacts retrieves Facts data from the database by ID (CIK or EIN)
func (db *DB) GetFacts(id string) (*facts.Facts, error) {
	query := "SELECT data FROM facts WHERE id = ?"
	
	var data []byte
	err := db.conn.QueryRow(query, id).Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("facts not found for ID %s", id)
		}
		return nil, fmt.Errorf("failed to query facts: %w", err)
	}

	var f facts.Facts
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("failed to unmarshal facts: %w", err)
	}

	return &f, nil
}

// AreFactsStale checks if facts for a given ID (CIK or EIN) are older than the specified duration
func (db *DB) AreFactsStale(id string, maxAge time.Duration) (bool, error) {
	query := "SELECT updated_at FROM facts WHERE id = ?"
	
	var updatedAt string
	err := db.conn.QueryRow(query, id).Scan(&updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return true, nil // No facts exist, consider stale
		}
		return false, fmt.Errorf("failed to query facts timestamp: %w", err)
	}

	// Parse the timestamp (SQLite CURRENT_TIMESTAMP returns RFC3339 format)
	timestamp, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return false, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	// Check if data is older than maxAge
	return time.Since(timestamp) > maxAge, nil
}

// ListFactsCIKs returns all CIKs that have facts stored (SEC data only)
func (db *DB) ListFactsCIKs() ([]string, error) {
	query := `SELECT id FROM facts WHERE source_type = 'SEC' ORDER BY company_name`
	
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query facts: %w", err)
	}
	defer rows.Close()

	var ciks []string
	for rows.Next() {
		var cik string
		if err := rows.Scan(&cik); err != nil {
			return nil, fmt.Errorf("failed to scan facts row: %w", err)
		}
		ciks = append(ciks, cik)
	}
	return ciks, nil
}

// ListFactsEINs returns all EINs that have facts stored (IRS data only)
func (db *DB) ListFactsEINs() ([]string, error) {
	query := `SELECT id FROM facts WHERE source_type = 'IRS' ORDER BY company_name`
	
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query facts: %w", err)
	}
	defer rows.Close()

	var eins []string
	for rows.Next() {
		var ein string
		if err := rows.Scan(&ein); err != nil {
			return nil, fmt.Errorf("failed to scan facts row: %w", err)
		}
		eins = append(eins, ein)
	}
	return eins, nil
}

// StoreIRSReturn stores raw IRS XML return data in the database
func (db *DB) StoreIRSReturn(ein, returnType, taxYear string, xmlData []byte) error {
	query := `
		INSERT OR REPLACE INTO irs_returns (ein, return_type, tax_year, xml_data, updated_at) 
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	`
	_, err := db.conn.Exec(query, ein, returnType, taxYear, xmlData)
	if err != nil {
		return fmt.Errorf("failed to store IRS return: %w", err)
	}

	return nil
}

// GetIRSReturn retrieves raw IRS XML return data from the database
func (db *DB) GetIRSReturn(ein string) ([]byte, error) {
	query := "SELECT xml_data FROM irs_returns WHERE ein = ?"
	
	var xmlData []byte
	err := db.conn.QueryRow(query, ein).Scan(&xmlData)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("IRS return not found for EIN %s", ein)
		}
		return nil, fmt.Errorf("failed to query IRS return: %w", err)
	}

	return xmlData, nil
}

// AreIRSReturnsStale checks if IRS return data for a given EIN is older than the specified duration
func (db *DB) AreIRSReturnsStale(ein string, maxAge time.Duration) (bool, error) {
	query := "SELECT updated_at FROM irs_returns WHERE ein = ?"
	
	var updatedAt string
	err := db.conn.QueryRow(query, ein).Scan(&updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return true, nil // No data exists, consider stale
		}
		return false, fmt.Errorf("failed to query IRS return timestamp: %w", err)
	}

	// Parse the timestamp (SQLite CURRENT_TIMESTAMP returns RFC3339 format)
	timestamp, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return false, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	// Check if data is older than maxAge
	return time.Since(timestamp) > maxAge, nil
}

// ListIRSReturnEINs returns all EINs that have IRS return data stored
func (db *DB) ListIRSReturnEINs() ([]string, error) {
	query := `SELECT ein FROM irs_returns ORDER BY ein`
	
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query IRS returns: %w", err)
	}
	defer rows.Close()

	var eins []string
	for rows.Next() {
		var ein string
		if err := rows.Scan(&ein); err != nil {
			return nil, fmt.Errorf("failed to scan IRS return row: %w", err)
		}
		eins = append(eins, ein)
	}
	return eins, nil
}

// SearchCacheItem represents a single search cache entry
type SearchCacheItem struct {
	Title      string
	Path       string
	SourceType string
}

// StoreSearchCacheItem stores a single search cache item
func (db *DB) StoreSearchCacheItem(title, path, sourceType string) error {
	query := `
		INSERT OR REPLACE INTO search_cache (title, path, source_type, updated_at) 
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`
	_, err := db.conn.Exec(query, title, path, sourceType)
	if err != nil {
		return fmt.Errorf("failed to store search cache item: %w", err)
	}
	return nil
}

// StoreSearchCacheItems stores multiple search cache items in batches
func (db *DB) StoreSearchCacheItems(items []SearchCacheItem) error {
	const batchSize = 1000
	
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}
		
		batch := items[i:end]
		if err := db.storeSearchCacheBatch(batch); err != nil {
			return fmt.Errorf("failed to store search cache batch: %w", err)
		}
	}
	
	return nil
}

// storeSearchCacheBatch stores a batch of search cache items in a single transaction
func (db *DB) storeSearchCacheBatch(items []SearchCacheItem) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO search_cache (title, path, source_type, updated_at) 
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()
	
	for _, item := range items {
		if _, err := stmt.Exec(item.Title, item.Path, item.SourceType); err != nil {
			return fmt.Errorf("failed to execute statement: %w", err)
		}
	}
	
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	return nil
}

// ClearSearchCache clears all search cache entries
func (db *DB) ClearSearchCache() error {
	query := "DELETE FROM search_cache"
	_, err := db.conn.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to clear search cache: %w", err)
	}
	return nil
}

// SearchCache performs FTS search on cached organizations
func (db *DB) SearchCache(query string, limit int) ([]struct {
	Title      string
	Path       string
	SourceType string
}, error) {
	// Use FTS5 prefix query with *
	prefixQuery := query + "*"
	sqlQuery := `
		SELECT title, path, source_type 
		FROM search_cache 
		WHERE search_cache MATCH ? 
		ORDER BY rank 
		LIMIT ?
	`
	
	rows, err := db.conn.Query(sqlQuery, prefixQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search cache: %w", err)
	}
	defer rows.Close()

	var results []struct {
		Title      string
		Path       string
		SourceType string
	}
	
	for rows.Next() {
		var result struct {
			Title      string
			Path       string
			SourceType string
		}
		if err := rows.Scan(&result.Title, &result.Path, &result.SourceType); err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		results = append(results, result)
	}
	
	return results, nil
}

// GetSearchCacheCount returns the number of items in the search cache
func (db *DB) GetSearchCacheCount() (int, error) {
	query := "SELECT COUNT(*) FROM search_cache"
	var count int
	err := db.conn.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get search cache count: %w", err)
	}
	return count, nil
}
