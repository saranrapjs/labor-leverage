package irs

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ozkatz/cloudzip/pkg/remote"
	"github.com/ozkatz/cloudzip/pkg/zipfile"
	"github.com/saranrapjs/deflate64"
	"github.com/saranrapjs/labor-leverage/pkg/irsform"
)

const (
	baseURL = "https://apps.irs.gov/pub/epostcard/990/xml"

	// How often to ask the IRS whether its indexes have changed. Checks are
	// conditional requests, so an unchanged index only costs a 304.
	indexCheckInterval = 24 * time.Hour
	// How soon to try again after a check fails.
	indexRetryInterval = time.Hour
)

type NonProfit struct {
	Name       string
	EIN        string
	ReturnID   string
	BatchID    string
	ObjectID   string
	ReturnType string
	// TaxPeriod is the end of the fiscal year covered by the return, as YYYYMM.
	TaxPeriod string
	// Year is the IRS processing year, i.e. which index/zip the return lives in.
	Year string
}

// newerThan reports whether np is a more recent return than other.
func (np NonProfit) newerThan(other NonProfit) bool {
	if np.TaxPeriod != other.TaxPeriod {
		return np.TaxPeriod > other.TaxPeriod
	}
	// Same tax period (e.g. an amended return): prefer the later submission.
	if np.Year != other.Year {
		return np.Year > other.Year
	}
	if len(np.ObjectID) != len(other.ObjectID) {
		return len(np.ObjectID) > len(other.ObjectID)
	}
	return np.ObjectID > other.ObjectID
}

type IRSClient struct {
	cacheDir string
	numYears int

	refreshing atomic.Bool

	mu         sync.RWMutex
	nonProfits []NonProfit
	byEIN      map[string]NonProfit
	nextCheck  time.Time
}

// NewIRSClient loads the IRS 990 indexes for the numYears most recent
// processing years (counting the current one), keeping the most recent
// supported return for each EIN. Indexes missing from the cache are
// downloaded up front; cached ones are checked for updates lazily, by
// MaybeRefresh.
func NewIRSClient(cacheDir string, numYears int) (*IRSClient, error) {
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

	client := &IRSClient{
		cacheDir: cacheDir,
		numYears: numYears,
	}

	for _, year := range client.years() {
		if _, err := os.Stat(client.cacheFile(year)); os.IsNotExist(err) {
			if _, err := client.fetchIndex(year); err != nil {
				log.Printf("Warning: IRS index for %s unavailable: %v", year, err)
			}
		}
	}

	if err := client.reload(); err != nil {
		return nil, err
	}

	return client, nil
}

// years returns the numYears most recent IRS processing years. A year's
// index only holds the returns the IRS processed that year, so the current
// year's is always partial, and a nonprofit that files annually is
// guaranteed to appear in the current or previous year's.
func (c *IRSClient) years() []string {
	var years []string
	now := time.Now().Year()
	for y := now - c.numYears + 1; y <= now; y++ {
		years = append(years, strconv.Itoa(y))
	}
	return years
}

// reload rebuilds the nonprofit list from the cached indexes.
func (c *IRSClient) reload() error {
	byEIN := map[string]NonProfit{}
	loaded := 0
	for _, year := range c.years() {
		if _, err := os.Stat(c.cacheFile(year)); os.IsNotExist(err) {
			continue
		}
		if _, err := loadCSV(c.cacheFile(year), year, byEIN); err != nil {
			log.Printf("Warning: failed to load IRS index for %s: %v", year, err)
			continue
		}
		loaded++
	}
	if loaded == 0 {
		return fmt.Errorf("no IRS indexes could be loaded")
	}

	nonProfits := make([]NonProfit, 0, len(byEIN))
	for _, np := range byEIN {
		nonProfits = append(nonProfits, np)
	}

	c.mu.Lock()
	c.byEIN = byEIN
	c.nonProfits = nonProfits
	c.mu.Unlock()

	log.Printf("Loaded %d nonprofits from %d IRS indexes", len(nonProfits), loaded)
	return nil
}

// MaybeRefresh starts a background check for updated indexes if one is due.
// Call it when a nonprofit is requested. The caller isn't held up: it sees
// the current data, and later callers see new data once the refresh finishes.
func (c *IRSClient) MaybeRefresh() {
	c.mu.RLock()
	due := !time.Now().Before(c.nextCheck)
	c.mu.RUnlock()
	if !due || !c.refreshing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer c.refreshing.Store(false)
		c.refresh()
	}()
}

// refresh re-downloads any indexes the IRS has changed, and reloads if so.
// If any check fails, the next one is scheduled sooner than usual.
func (c *IRSClient) refresh() {
	changed, failed := false, false
	for _, year := range c.years() {
		updated, err := c.fetchIndex(year)
		if err != nil {
			log.Printf("Warning: failed to check IRS index for %s: %v", year, err)
			failed = true
			continue
		}
		changed = changed || updated
	}

	next := indexCheckInterval
	if failed {
		next = indexRetryInterval
	}
	c.mu.Lock()
	c.nextCheck = time.Now().Add(next)
	c.mu.Unlock()

	if changed {
		if err := c.reload(); err != nil {
			log.Printf("Warning: failed to reload IRS indexes: %v", err)
		}
	} else if !failed {
		log.Printf("IRS indexes are up to date")
	}
}

// NonProfits returns the most recent supported return for every known EIN.
func (c *IRSClient) NonProfits() []NonProfit {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nonProfits
}

// Lookup returns the most recent supported return for an EIN.
func (c *IRSClient) Lookup(ein string) (NonProfit, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	np, ok := c.byEIN[ein]
	return np, ok
}

func (c *IRSClient) cacheFile(year string) string {
	return filepath.Join(c.cacheDir, fmt.Sprintf("irs_index_%s.csv", year))
}

// loadCSV reads the supported returns in an index file into byEIN, keeping
// the most recent per EIN, and reports how many supported returns it read.
func loadCSV(path, year string, byEIN map[string]NonProfit) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("failed to open cache file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return 0, fmt.Errorf("failed to read CSV header: %w", err)
	}

	cols := map[string]int{}
	for i, col := range header {
		cols[strings.TrimSpace(col)] = i
	}
	required := []string{"TAXPAYER_NAME", "EIN", "RETURN_ID", "XML_BATCH_ID", "OBJECT_ID", "RETURN_TYPE", "TAX_PERIOD"}
	maxCol := 0
	for _, name := range required {
		i, ok := cols[name]
		if !ok {
			return 0, fmt.Errorf("required column %s not found in CSV", name)
		}
		maxCol = max(maxCol, i)
	}

	count := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("failed to read CSV: %w", err)
		}
		if len(record) <= maxCol {
			continue
		}

		np := NonProfit{
			Name:       record[cols["TAXPAYER_NAME"]],
			EIN:        record[cols["EIN"]],
			ReturnID:   record[cols["RETURN_ID"]],
			BatchID:    record[cols["XML_BATCH_ID"]],
			ObjectID:   record[cols["OBJECT_ID"]],
			ReturnType: record[cols["RETURN_TYPE"]],
			TaxPeriod:  record[cols["TAX_PERIOD"]],
			Year:       year,
		}
		if !irsform.IsSupportedReturnType(np.ReturnType) {
			continue
		}
		count++
		if existing, ok := byEIN[np.EIN]; ok && !np.newerThan(existing) {
			continue
		}
		byEIN[np.EIN] = np
	}

	return count, nil
}

// fetchIndex downloads a year's index if the IRS has changed it since the
// cached copy, reporting whether it did. The cached file's mtime is set to
// the IRS's Last-Modified, and sent back as If-Modified-Since.
func (c *IRSClient) fetchIndex(year string) (bool, error) {
	indexURL := fmt.Sprintf("%s/%s/index_%s.csv", baseURL, year, year)
	cacheFile := c.cacheFile(year)

	req, err := http.NewRequest(http.MethodGet, indexURL, nil)
	if err != nil {
		return false, err
	}
	if info, err := os.Stat(cacheFile); err == nil {
		req.Header.Set("If-Modified-Since", info.ModTime().UTC().Format(http.TimeFormat))
	}

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to fetch CSV: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	log.Printf("Downloading IRS index %s", indexURL)

	// Write to a temp file first so a failed download never clobbers a good cache.
	tmp, err := os.CreateTemp(c.cacheDir, fmt.Sprintf("irs_index_%s_*.csv.tmp", year))
	if err != nil {
		return false, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return false, fmt.Errorf("failed to write cache file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("failed to write cache file: %w", err)
	}
	// Make sure the download parses before it replaces the cached copy.
	if n, err := loadCSV(tmp.Name(), year, map[string]NonProfit{}); err != nil {
		return false, fmt.Errorf("downloaded index is invalid: %w", err)
	} else if n == 0 {
		return false, fmt.Errorf("downloaded index has no supported returns")
	}
	// CreateTemp makes files 0600; match what os.Create would have made.
	if err := os.Chmod(tmp.Name(), 0644); err != nil {
		return false, fmt.Errorf("failed to set cache file permissions: %w", err)
	}

	if err := os.Rename(tmp.Name(), cacheFile); err != nil {
		return false, fmt.Errorf("failed to move cache file into place: %w", err)
	}
	if lastModified, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		if err := os.Chtimes(cacheFile, lastModified, lastModified); err != nil {
			log.Printf("Warning: failed to set mtime on %s: %v", cacheFile, err)
		}
	}

	return true, nil
}

func (c *IRSClient) FetchCompany(ein string) ([]byte, error) {
	nonprofit, ok := c.Lookup(ein)
	if !ok {
		return nil, fmt.Errorf("EIN %s not found", ein)
	}

	filename := fmt.Sprintf("%s_public.xml", nonprofit.ObjectID)
	var lastErr error
	for _, batchID := range candidateBatches(nonprofit.BatchID) {
		data, err := fetchFromZip(nonprofit.Year, batchID, filename)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// candidateBatches returns the zips a return might live in. The index isn't
// always right about this: e.g. every 2025_TEOS_XML_05B return is listed in
// the index as 2025_TEOS_XML_05A. So after the indexed batch, try the other
// lettered batches from the same month.
func candidateBatches(batchID string) []string {
	batchID = strings.ToUpper(batchID)
	candidates := []string{batchID}
	last := len(batchID) - 1
	if last < 0 || batchID[last] < 'A' || batchID[last] > 'Z' {
		return candidates
	}
	for letter := byte('A'); letter <= 'D'; letter++ {
		if letter != batchID[last] {
			candidates = append(candidates, batchID[:last]+string(letter))
		}
	}
	return candidates
}

// fetchFromZip reads one file out of a remote IRS zip using range requests.
// Zips from 2024 and earlier nest files under a directory named after the
// batch, while later ones don't, so files are matched on their base name.
func fetchFromZip(year, batchID, filename string) ([]byte, error) {
	zipURL := fmt.Sprintf("%s/%s/%s.zip", baseURL, year, batchID)

	fetcher, err := remote.NewHttpFetcher(zipURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP fetcher: %w", err)
	}
	adapter := zipfile.NewStorageAdapter(context.Background(), fetcher)
	parser := zipfile.NewCentralDirectoryParser(adapter)
	directory, err := parser.GetCentralDirectory()
	if err != nil {
		return nil, fmt.Errorf("failed to read ZIP directory %s: %w", zipURL, err)
	}

	for _, record := range directory {
		if path.Base(record.FileName) != filename {
			continue
		}
		// cloudzip only inflates plain Deflate, and passes anything else
		// through as-is, so Deflate64 (used for batches over ~2GiB since
		// 2025) has to be decompressed here.
		reader, err := zipfile.ReaderForRecord(record, adapter)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s from ZIP %s: %w", record.FileName, zipURL, err)
		}
		switch record.CompressionMethod {
		case zip.Store, zip.Deflate:
		case deflate64.ZipMethod:
			reader = deflate64.NewReader(reader)
		default:
			return nil, fmt.Errorf("unsupported compression method %d for %s in ZIP %s", record.CompressionMethod, record.FileName, zipURL)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to read file contents: %w", err)
		}
		if crc32.ChecksumIEEE(data) != record.CRC32Uncompressed {
			return nil, fmt.Errorf("checksum mismatch for %s in ZIP %s", record.FileName, zipURL)
		}
		return data, nil
	}

	return nil, fmt.Errorf("file %s not found in ZIP %s", filename, zipURL)
}
