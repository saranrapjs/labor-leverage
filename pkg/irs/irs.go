package irs

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ozkatz/cloudzip/pkg/remote"
	"github.com/ozkatz/cloudzip/pkg/zipfile"
	"github.com/saranrapjs/labor-leverage/pkg/irsform"
)

const (
	baseURL = "https://apps.irs.gov/pub/epostcard/990/xml"
)

type NonProfit struct {
	Name     string
	EIN      string
	ReturnID string
	BatchID  string
	ObjectID string
	ReturnType string
}

type IRSClient struct {
	cacheFile string
	year      string
	NonProfits []NonProfit
}

func NewIRSClient(cacheDir, year string) (*IRSClient, error) {
	if cacheDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		cacheDir = filepath.Join(homeDir, ".cache", "labor-leverage")
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	cacheFile := filepath.Join(cacheDir, fmt.Sprintf("irs_index_%s.csv", year))
	client := &IRSClient{
		cacheFile: cacheFile,
		year:      year,
	}

	if err := client.loadCSV(); err != nil {
		return nil, fmt.Errorf("failed to load CSV: %w", err)
	}

	return client, nil
}

func (c *IRSClient) loadCSV() error {
	if _, err := os.Stat(c.cacheFile); os.IsNotExist(err) {
		if err := c.fetchAndCacheCSV(); err != nil {
			return err
		}
	}

	file, err := os.Open(c.cacheFile)
	if err != nil {
		return fmt.Errorf("failed to open cache file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read CSV: %w", err)
	}

	if err := c.parseRecords(records); err != nil {
		return fmt.Errorf("failed to parse records: %w", err)
	}

	return nil
}

func (c *IRSClient) parseRecords(records [][]string) error {
	if len(records) == 0 {
		return fmt.Errorf("no records found")
	}

	header := records[0]
	nameCol := -1
	einCol := -1
	returnIDCol := -1
	xmlBatchIDCol := -1
	objectIDCol := -1
	returnTypeCol := -1

	for i, col := range header {
		switch col {
		case "TAXPAYER_NAME":
			nameCol = i
		case "EIN":
			einCol = i
		case "RETURN_ID":
			returnIDCol = i
		case "XML_BATCH_ID":
			xmlBatchIDCol = i
		case "OBJECT_ID":
			objectIDCol = i
		case "RETURN_TYPE":
			returnTypeCol = i
		}
	}

	if nameCol == -1 || einCol == -1 || returnIDCol == -1 || xmlBatchIDCol == -1 || objectIDCol == -1 || returnTypeCol == -1 {
		return fmt.Errorf("required columns not found in CSV")
	}

	nonprofits := make([]NonProfit, 0, len(records)-1)
	for i := 1; i < len(records); i++ {
		record := records[i]
		if len(record) > nameCol && len(record) > einCol && len(record) > returnIDCol && len(record) > xmlBatchIDCol && len(record) > objectIDCol {
			nonprofits = append(nonprofits, NonProfit{
				Name:     record[nameCol],
				EIN:      record[einCol],
				ReturnID: record[returnIDCol],
				BatchID:  record[xmlBatchIDCol],
				ObjectID: record[objectIDCol],
				ReturnType: record[returnTypeCol],
			})
		}
	}

	c.NonProfits = nonprofits
	return nil
}

func (c *IRSClient) fetchAndCacheCSV() error {
	indexURL := fmt.Sprintf("https://apps.irs.gov/pub/epostcard/990/xml/%s/index_%s.csv", c.year, c.year)
	
	resp, err := http.Get(indexURL)
	if err != nil {
		return fmt.Errorf("failed to fetch CSV: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	file, err := os.Create(c.cacheFile)
	if err != nil {
		return fmt.Errorf("failed to create cache file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

func (c *IRSClient) FetchCompany(ein string) ([]byte, error) {
	if len(c.NonProfits) == 0 {
		return nil, fmt.Errorf("no nonprofit data loaded")
	}

	var nonprofit *NonProfit
	for _, np := range c.NonProfits {
		if strings.EqualFold(np.EIN, ein) && irsform.IsSupportedReturnType(np.ReturnType) {
			nonprofit = &np
			break
		}
	}

	if nonprofit == nil {
		return nil, fmt.Errorf("EIN %s not found", ein)
	}

	batchID := strings.ToUpper(nonprofit.BatchID)
	zipURL := fmt.Sprintf("%s/%s/%s.zip", baseURL, c.year, batchID)
	
	ctx := context.Background()
	
	fetcher, err := remote.NewHttpFetcher(zipURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP fetcher: %w", err)
	}
	adapter := zipfile.NewStorageAdapter(ctx, fetcher)
	parser := zipfile.NewCentralDirectoryParser(adapter)
	filename := fmt.Sprintf("%s/%s_public.xml", batchID, nonprofit.ObjectID)
	// fmt.Println("looking for", filename, zipURL)
	reader, err := parser.Read(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s from ZIP: %w", filename, err)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file contents: %w", err)
	}

	return data, nil
}
