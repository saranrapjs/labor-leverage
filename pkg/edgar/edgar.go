package edgar

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

//go:embed tickers.json
var tickersJSON []byte

type TickerData struct {
	CIKStr int    `json:"cik_str"`
	Ticker string `json:"ticker"`
	Title  string `json:"title"`
}

var TickersData map[string]TickerData

func init() {
	if err := json.Unmarshal(tickersJSON, &TickersData); err != nil {
		panic(fmt.Sprintf("failed to parse tickers data: %v", err))
	}
}

type Document struct {
	Filing
	DocumentFile []byte
}

type Filing struct {
	CIK                   string
	AccessionNumber       string `json:"accessionNumber"`
	FilingDate            string `json:"filingDate"`
	ReportDate            string `json:"reportDate"`
	Form                  string `json:"form"`
	FileNumber            string `json:"fileNumber"`
	IsXBRL                int    `json:"isXBRL"`
	IsInlineXBLR          int    `json:"isInlineXBRL"`
	PrimaryDocument       string `json:"primaryDocument"`
	PrimaryDocDescription string `json:"primaryDocDescription"`
}

func (f Filing) URL() string {
	accessionNumber := strings.ReplaceAll(f.AccessionNumber, "-", "")
	
	return fmt.Sprintf("https://www.sec.gov/Archives/edgar/data/%s/%s/%s", 
		f.CIK, accessionNumber, f.PrimaryDocument)
}

type Filings struct {
	Recent struct {
		AccessionNumber       []string `json:"accessionNumber"`
		FilingDate            []string `json:"filingDate"`
		ReportDate            []string `json:"reportDate"`
		Form                  []string `json:"form"`
		FileNumber            []string `json:"fileNumber"`
		IsXBRL                []int    `json:"isXBRL"`
		IsInlineXBLR          []int    `json:"isInlineXBRL"`
		PrimaryDocument       []string `json:"primaryDocument"`
		PrimaryDocDescription []string `json:"primaryDocDescription"`
	} `json:"recent"`
}

func (f Filings) Index(i int) Filing {
	return Filing{
		AccessionNumber      : f.Recent.AccessionNumber[i],
		FilingDate           : f.Recent.FilingDate[i], 
		ReportDate           : f.Recent.ReportDate[i],
		Form                 : f.Recent.Form[i],
		FileNumber           : f.Recent.FileNumber[i],
		IsXBRL               : f.Recent.IsXBRL[i],
		IsInlineXBLR         : f.Recent.IsInlineXBLR[i],
		PrimaryDocument      : f.Recent.PrimaryDocument[i],
		PrimaryDocDescription: f.Recent.PrimaryDocDescription[i],
	}
}

func (f Filings) Search(cik, formName string) (Filing, bool) {
	for i, name := range f.Recent.Form {
		if strings.Contains(name, formName) {
			filing := f.Index(i)
			filing.CIK = cik
			return filing, true
		}
	}
	return Filing{}, false
}

type Submissions struct {
	CIK       string   `json:"cik"`
	Name      string   `json:"name"`
	Tickers   []string `json:"tickers"`
	Exchanges []string `json:"exchanges"`
	Filings   Filings  `json:"filings"`
}

// Ticker2CIK returns the CIK string for a given ticker symbol
func Ticker2CIK(ticker string) (string, error) {
	// Search for the ticker symbol in the pre-parsed data
	for _, data := range TickersData {
		if data.Ticker == ticker {
			// Convert CIK to string
			return strconv.Itoa(data.CIKStr), nil
		}
	}
	return "", fmt.Errorf("ticker %s not found", ticker)
}

func CIK2Ticker(cik string) (string, error) {
	// Search for the ticker symbol in the pre-parsed data
	for _, data := range TickersData {
		cikStr := strconv.Itoa(data.CIKStr)
		if cik == cikStr {
			// Convert CIK to string
			return data.Ticker, nil
		}
	}
	return "", fmt.Errorf("ticker %v not found", cik)
}

// Ticker2CompanyName returns the company title for a given ticker symbol
func Ticker2CompanyName(ticker string) (string, error) {
	// Search for the ticker symbol in the pre-parsed data
	for _, data := range TickersData {
		if data.Ticker == ticker {
			return data.Title, nil
		}
	}
	return "", fmt.Errorf("ticker %s not found", ticker)
}
