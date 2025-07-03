package facts

import (
	"bytes"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/saranrapjs/labor-leverage/pkg/edgar"
	"github.com/saranrapjs/labor-leverage/pkg/ixbrl"
)

// Facts represents the transformed data extracted from Edgar filings
type Facts struct {
	CIK                  string               `json:"cik"`
	Ticker               string               `json:"ticker"`
	CompanyName          string               `json:"company_name"`
	Filings              []edgar.Filing       `json:"filings"`
	NetIncomeLoss        []*ixbrl.NonFraction `json:"net_revenue"`
	Buybacks             []*ixbrl.NonFraction `json:"buybacks"`
	ExecCompensationHTML []string             `json:"exec_compensation_html"`
	CEOPayRatio          *CEOPayRatio          `json:"ceo_pay_ratio"`
	Cash                 []*ixbrl.NonFraction `json:"cash"`
	EmployeesCount       int                  `json:"employees_count"`
}

// ExtractFacts processes Edgar filing documents and extracts Facts data
func ExtractFacts(cik, ticker, companyName string, filingDocs []edgar.Document) (*Facts, error) {
	facts := &Facts{
		CIK:         cik,
		Ticker:      ticker,
		CompanyName: companyName,
	}

	for _, f := range filingDocs {
		r := bytes.NewReader(f.DocumentFile)
		p, doc, err := ixbrl.Parse(r)
		if err != nil {
			return nil, err
		}

		// Extract stock repurchases
		stockRepurchase := ixbrl.Search(p, func(f *ixbrl.NonFraction) bool {
			return f.Name == "us-gaap:StockRepurchasedDuringPeriodValue"
		})
		if stockRepurchase != nil {
			facts.Buybacks = append(facts.Buybacks, stockRepurchase)
		}

		// Extract net revenue
		netRevenue := ixbrl.Search(p, func(f *ixbrl.NonFraction) bool {
			return f.Name == "us-gaap:NetIncomeLoss"
		})
		if netRevenue != nil {
			facts.NetIncomeLoss = append(facts.NetIncomeLoss, netRevenue)
		}

		// Extract cash
		cash := ixbrl.Search(p, func(f *ixbrl.NonFraction) bool {
			return f.Name == "us-gaap:CashCashEquivalentsRestrictedCashAndRestrictedCashEquivalents"
		})
		if cash != nil {
			facts.Cash = append(facts.Cash, cash)
		}

		// Extract CEO pay ratio
		ratios := ixbrl.SearchHTML(doc, func(t string) string {
			if strings.Contains(strings.ToLower(t), "ceo pay ratio") {
				return t
			}
			return ""
		})
		for _, m := range ratios {
			leafText := ixbrl.FindNextLeafNodes(m.Node, 700)
			if strings.Contains(leafText, "$") && strings.Contains(leafText, "median") {
				ceoRatio := extractCEOPayRatio(leafText)
				if facts.CEOPayRatio == nil && ceoRatio.Text != "" {
					facts.CEOPayRatio = &ceoRatio
					break
				}
			}
		}

		// Extract employee count
		e := regexp.MustCompile("([\\d]{1}[\\d,]{1,})[^.,%]*employees")
		employees := ixbrl.SearchHTML(doc, func(t string) string {
			lowered := strings.ToLower(t)
			if strings.Contains(lowered, "december") {
				match := e.FindAllStringSubmatch(t, -1)
				if match != nil {
					matchedGroup := match[0][1]
					if !strings.HasPrefix(matchedGroup, "20") || len(matchedGroup) != 4 {
						return matchedGroup
					}
				}
			}
			return ""
		})
		for _, m := range employees {
			if facts.EmployeesCount == 0 {
				facts.EmployeesCount = onlyNumber(m.Text)
			}
		}

		// Extract executive compensation tables
		tables := ixbrl.FindTables(doc, func(text string) bool {
			return strings.Contains(text, "Name") && strings.Contains(text, "$") && strings.Contains(text, "Salary")
		})
		for _, t := range tables {
			facts.ExecCompensationHTML = append(facts.ExecCompensationHTML, ixbrl.Print(t))
		}

		facts.Filings = append(facts.Filings, f.Filing)
	}

	// Sort all NonFraction slices in reverse chronological order
	sortNonFractionsByDate(facts.NetIncomeLoss)
	sortNonFractionsByDate(facts.Buybacks)
	sortNonFractionsByDate(facts.Cash)

	return facts, nil
}

type CEOPayRatio struct {
	Text string
	CEO float64
	Median float64
}

// extractCEOPayRatio extracts two dollar amounts from text and formats them as CEO vs median
func extractCEOPayRatio(text string) CEOPayRatio {
	// Regex to find dollar amounts (including commas and decimals)
	dollarRegex := regexp.MustCompile(`\$[\d,]+(?:\.\d{2})?`)
	matches := dollarRegex.FindAllString(text, -1)
	
	if len(matches) < 2 {
		return CEOPayRatio{Text:text}
	}
	
	var amounts []float64
	var amountStrs []string
	
	// Parse each dollar amount
	for _, match := range matches {
		// Remove $ and commas
		cleanAmount := strings.ReplaceAll(strings.TrimPrefix(match, "$"), ",", "")
		amount, err := strconv.ParseFloat(cleanAmount, 64)
		if err != nil {
			continue
		}
		amounts = append(amounts, amount)
		amountStrs = append(amountStrs, match)
	}
	
	if len(amounts) < 2 {
		return CEOPayRatio{Text:text}
	}
	
	// Find highest and lowest amounts
	var ceoVal, medianVal float64
	
	ceoVal = amounts[0]
	medianVal = amounts[0]

	for _, amount := range amounts {
		if amount > ceoVal {
			ceoVal = amount
		}
		if amount < medianVal {
			medianVal = amount
		}
	}
	
	// Format the result
	return CEOPayRatio{text, ceoVal, medianVal}
}

// sortNonFractionsByDate sorts a slice of NonFraction in reverse chronological order
func sortNonFractionsByDate(nfs []*ixbrl.NonFraction) {
	sort.Slice(nfs, func(i, j int) bool {
		dateI := getLatestDate(nfs[i])
		dateJ := getLatestDate(nfs[j])
		return dateI.After(dateJ) // Reverse chronological (newest first)
	})
}

// getLatestDate extracts the latest date from a NonFraction's context
func getLatestDate(nf *ixbrl.NonFraction) time.Time {
	if nf.Context == nil {
		return time.Time{} // Zero time for entries without context
	}
	
	period := nf.Context.Period
	
	// Try EndDate first (for ranges), then Instant
	dateStr := period.EndDate
	if dateStr == "" {
		dateStr = period.Instant
	}
	if dateStr == "" {
		dateStr = period.StartDate
	}
	
	// Parse the date (IXBRL typically uses YYYY-MM-DD format)
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{} // Zero time for unparseable dates
	}
	
	return date
}

// onlyNumber extracts numeric characters from a string and returns as int
func onlyNumber(str string) int {
	var numericChars strings.Builder

	// Filter out non-numeric characters
	for _, char := range str {
		if unicode.IsDigit(char) {
			numericChars.WriteRune(char)
		}
	}

	// Get the filtered string
	numericStr := numericChars.String()

	// Handle empty string case
	if numericStr == "" {
		return 0
	}

	// Parse as integer
	result, err := strconv.Atoi(numericStr)
	if err != nil {
		return 0
	}

	return result
}
