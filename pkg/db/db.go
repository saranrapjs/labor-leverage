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

	// Create facts table
	factsSQL := `
		CREATE TABLE IF NOT EXISTS facts (
			cik TEXT PRIMARY KEY,
			ticker TEXT NOT NULL,
			data BLOB NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`
	if _, err := db.conn.Exec(factsSQL); err != nil {
		return fmt.Errorf("failed to create facts table: %w", err)
	}

	// Add company_name column if it doesn't exist (migration)
	addColumnSQL := `
		ALTER TABLE facts ADD COLUMN company_name TEXT DEFAULT '';
	`
	// Ignore error if column already exists
	db.conn.Exec(addColumnSQL)

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

	// Insert or replace the facts data
	query := `
		INSERT OR REPLACE INTO facts (cik, ticker, company_name, data, updated_at) 
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	`
	_, err = db.conn.Exec(query, f.CIK, f.Ticker, f.CompanyName, data)
	if err != nil {
		return fmt.Errorf("failed to store facts: %w", err)
	}

	return nil
}

// GetFacts retrieves Facts data from the database
func (db *DB) GetFacts(cik string) (*facts.Facts, error) {
	query := "SELECT data FROM facts WHERE cik = ?"
	
	var data []byte
	err := db.conn.QueryRow(query, cik).Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("facts not found for CIK %s", cik)
		}
		return nil, fmt.Errorf("failed to query facts: %w", err)
	}

	var f facts.Facts
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("failed to unmarshal facts: %w", err)
	}

	return &f, nil
}

// AreFactsStale checks if facts for a given CIK are older than the specified duration
func (db *DB) AreFactsStale(cik string, maxAge time.Duration) (bool, error) {
	query := "SELECT updated_at FROM facts WHERE cik = ?"
	
	var updatedAt string
	err := db.conn.QueryRow(query, cik).Scan(&updatedAt)
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

// ListFactsCIKs returns all CIKs that have facts stored
func (db *DB) ListFactsCIKs() ([]string, error) {
	query := `SELECT cik FROM facts ORDER BY ticker`
	
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
