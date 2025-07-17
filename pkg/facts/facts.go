package facts

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/saranrapjs/labor-leverage/pkg/edgar"
	"github.com/saranrapjs/labor-leverage/pkg/irsform"
	"github.com/saranrapjs/labor-leverage/pkg/ixbrl"
	"golang.org/x/text/message"
)

// Facts represents the transformed data extracted from Edgar filings or IRS returns
type Facts struct {
	CIK                  string               `json:"cik"`
	EIN                  string               `json:"ein,omitempty"`
	Ticker               string               `json:"ticker,omitempty"`
	CompanyName          string               `json:"company_name"`
	Filings              []edgar.Filing       `json:"filings,omitempty"`
	NetIncomeLoss        []*ixbrl.NonFraction `json:"net_revenue,omitempty"`
	Buybacks             []*ixbrl.NonFraction `json:"buybacks,omitempty"`
	ExecCompensationHTML []string             `json:"exec_compensation_html,omitempty"`
	CEOPayRatio          *CEOPayRatio          `json:"ceo_pay_ratio,omitempty"`
	Cash                 []*ixbrl.NonFraction `json:"cash,omitempty"`
	EmployeesCount       int                  `json:"employees_count"`
	TotalRevenue         int                  `json:"total_revenue,omitempty"`
	TotalExpenses        int                  `json:"total_expenses,omitempty"`
	NetAssets            *ixbrl.NonFraction   `json:"net_assets,omitempty"`
	WorkerPay            []*ixbrl.NonFraction   `json:"worker_pay,omitempty"`
}

// FromEdgar processes Edgar filing documents and extracts Facts data
func FromEdgar(cik, ticker, companyName string, filingDocs []edgar.Document) (*Facts, error) {
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

func valueToIxFraction(val int, start, end string) *ixbrl.NonFraction {
	return &ixbrl.NonFraction{
		Scale: "0",
		Content: strconv.Itoa(val),
		Context: &ixbrl.Context{
			Period: ixbrl.Period{
				StartDate: start,
				EndDate: end,
			},
		},
	}
}

var printer = message.NewPrinter(message.MatchLanguage("en"))


func irsExecComp(execs []*irsform.Form990PartVIISectionAGrp) string {
	var b strings.Builder
	b.WriteString(`<table style="font-family:monospace;"><thead><tr>
		<th>Name</th>
		<th>Title</th>
		<th>Compensation</th>
</tr></thead><tbody>`)
	for _, e := range execs {
		b.WriteString(fmt.Sprintf(`<tr>
			<td>%s</td>
			<td>%s</td>
			<td>%s</td>
		</tr>`, e.PersonNm, e.TitleTxt, printer.Sprintf("$%d", e.ReportableCompFromOrgAmt + e.OtherCompensationAmt)))
	}
	b.WriteString("</tbody></table>")
	return b.String()
}

const layout = "2006-01-02"

func minusOneYear(date string) (string, string) {
	t, err := time.Parse(layout, date)
	if err != nil {
		return "", ""
	}
	return t.Add(-1 * time.Hour * 24 * 365).Format(layout), t.Add(-1 * time.Hour * 24).Format(layout)
}

// FromIRS processes IRS return data and extracts Facts data
func FromIRS(returnDoc *irsform.Return) (*Facts, error) {
	if returnDoc == nil {
		return nil, fmt.Errorf("invalid return data: nil return document")
	}

	facts := &Facts{}

	// Extract company name from ReturnHeader
	if returnDoc.ReturnHeader.Filer.BusinessName.BusinessNameLine1Txt != "" {
		facts.CompanyName = returnDoc.ReturnHeader.Filer.BusinessName.BusinessNameLine1Txt
	}

	// Handle different return types
	switch data := returnDoc.ReturnData.(type) {
	case *irsform.ReturnData990:
		if data.IRS990 == nil {
			return nil, fmt.Errorf("invalid return data: missing IRS990")
		}
		irs990 := data.IRS990
		facts.EmployeesCount = irs990.TotalEmployeeCnt
		facts.NetIncomeLoss = append(facts.NetIncomeLoss, valueToIxFraction(irs990.CYTotalRevenueAmt - irs990.CYTotalExpensesAmt, returnDoc.ReturnHeader.TaxPeriodBeginDt, returnDoc.ReturnHeader.TaxPeriodEndDt))

		// facts.TotalRevenue = irs990.CYTotalRevenueAmt
		// facts.TotalExpenses = irs990.CYTotalExpensesAmt
		facts.NetAssets = valueToIxFraction(irs990.NetAssetsOrFundBalancesEOYAmt, returnDoc.ReturnHeader.TaxPeriodBeginDt, returnDoc.ReturnHeader.TaxPeriodEndDt)

		// Use principal officer business name if available and ReturnHeader name is empty
		if facts.CompanyName == "" && irs990.PrincipalOfcrBusinessName != nil && irs990.PrincipalOfcrBusinessName.BusinessNameLine1Txt != "" {
			facts.CompanyName = irs990.PrincipalOfcrBusinessName.BusinessNameLine1Txt
		}
		facts.ExecCompensationHTML = append(facts.ExecCompensationHTML, irsExecComp(irs990.Form990PartVIISectionAGrp))
		facts.WorkerPay = append(facts.WorkerPay, valueToIxFraction(irs990.CYSalariesCompEmpBnftPaidAmt, returnDoc.ReturnHeader.TaxPeriodBeginDt, returnDoc.ReturnHeader.TaxPeriodEndDt))
		previousYearStart, previousYearEnd := minusOneYear(returnDoc.ReturnHeader.TaxPeriodBeginDt)
		facts.WorkerPay = append(facts.WorkerPay, valueToIxFraction(irs990.PYSalariesCompEmpBnftPaidAmt, previousYearStart, previousYearEnd))
	case *irsform.ReturnData990EZ:
		if data.IRS990EZ == nil {
			return nil, fmt.Errorf("invalid return data: missing IRS990EZ")
		}
		// Cast IRS990EZ from interface{} to the actual type
		irs990ez := data.IRS990EZ
		facts.NetIncomeLoss = append(facts.NetIncomeLoss, valueToIxFraction(irs990ez.TotalRevenueAmt - irs990ez.TotalExpensesAmt, returnDoc.ReturnHeader.TaxPeriodBeginDt, returnDoc.ReturnHeader.TaxPeriodEndDt))
		facts.NetAssets = valueToIxFraction(irs990ez.NetAssetsOrFundBalancesEOYAmt, returnDoc.ReturnHeader.TaxPeriodBeginDt, returnDoc.ReturnHeader.TaxPeriodEndDt)
	case *irsform.ReturnData990PF:
		if data.IRS990PF == nil {
			return nil, fmt.Errorf("invalid return data: missing IRS990PF")
		}
		// TODO!
	default:
		return nil, fmt.Errorf("unsupported return type: %T", data)
	}
	sortNonFractionsByDate(facts.NetIncomeLoss)
	sortNonFractionsByDate(facts.Buybacks)
	sortNonFractionsByDate(facts.Cash)
	return facts, nil
}

// ExtractFacts is a backwards compatibility wrapper for FromEdgar
func ExtractFacts(cik, ticker, companyName string, filingDocs []edgar.Document) (*Facts, error) {
	return FromEdgar(cik, ticker, companyName, filingDocs)
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
