package facts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/saranrapjs/labor-leverage/pkg/edgar"
	"github.com/saranrapjs/labor-leverage/pkg/irsform"
)

func TestFactsPackageDualPurpose(t *testing.T) {
	t.Run("Edgar Facts Extraction", func(t *testing.T) {
		// Test Edgar functionality with backwards compatibility
		fixturesDir := filepath.Join(".", "fixtures")
		htmlPath := filepath.Join(fixturesDir, "apple.html")
		
		htmlContent, err := os.ReadFile(htmlPath)
		require.NoError(t, err, "Failed to read fixture file")

		// Create mock Edgar document
		doc := edgar.Document{
			DocumentFile: htmlContent,
			Filing:       edgar.Filing{},
		}

		// Test ExtractFacts (backwards compatibility)
		facts, err := ExtractFacts("test-cik", "TEST", "Test Company", []edgar.Document{doc})
		require.NoError(t, err, "Failed to extract facts")
		
		assert.Equal(t, "test-cik", facts.CIK)
		assert.Equal(t, "TEST", facts.Ticker)
		assert.Equal(t, "Test Company", facts.CompanyName)

		// Test FromEdgar directly
		factsFromEdgar, err := FromEdgar("test-cik", "TEST", "Test Company", []edgar.Document{doc})
		require.NoError(t, err, "Failed to extract facts using FromEdgar")
		
		// Both should produce the same results
		assert.Equal(t, facts.CIK, factsFromEdgar.CIK)
		assert.Equal(t, facts.Ticker, factsFromEdgar.Ticker)
		assert.Equal(t, facts.CompanyName, factsFromEdgar.CompanyName)
	})

	t.Run("IRS Facts Extraction", func(t *testing.T) {
		// Create mock IRS990 data instead of relying on XML parsing
		mockIRS990 := &irsform.IRS990Type{
			PrincipalOfficerNm:            "Jane Smith",
			TotalEmployeeCnt:              150,
			CYTotalRevenueAmt:             3000000,
			CYTotalExpensesAmt:            2800000,
			NetAssetsOrFundBalancesEOYAmt: 800000,
		}

		// Create mock ReturnData990
		returnData990 := &irsform.ReturnData990{
			IRS990: &irsform.IRS990{
				IRS990Type: mockIRS990,
			},
		}

		// Create mock Return document
		returnDoc := &irsform.Return{
			ReturnHeader: irsform.ReturnHeader{
				ReturnTypeCd: "990",
				Filer: irsform.Filer{
					BusinessName: irsform.BusinessNameType{
						BusinessNameLine1Txt: "Test IRS Organization",
					},
				},
			},
			ReturnData: returnData990,
		}

		// Extract facts using FromIRS
		facts, err := FromIRS(returnDoc)
		require.NoError(t, err, "Failed to extract facts from IRS data")

		// Verify IRS-specific fields are populated
		assert.Equal(t, "Test IRS Organization", facts.CompanyName, "Company name should be extracted from ReturnHeader")
		assert.Equal(t, 150, facts.EmployeesCount, "Employee count should match")
		
		// Verify Edgar-specific fields are not populated
		assert.Empty(t, facts.CIK, "CIK should be empty for IRS data")
		assert.Empty(t, facts.Ticker, "Ticker should be empty for IRS data")
		assert.Empty(t, facts.Buybacks, "Buybacks should be empty for IRS data")
	})
}

func TestFactsStructFields(t *testing.T) {
	// Test that Facts struct can handle both Edgar and IRS data appropriately
	facts := &Facts{
		CIK:           "test-cik",
		EIN:           "test-ein", 
		Ticker:        "TEST",
		CompanyName:   "Test Company",
		EmployeesCount: 100,
		TotalRevenue:  1000000,
		TotalExpenses: 800000,
	}

	assert.Equal(t, "test-cik", facts.CIK)
	assert.Equal(t, "test-ein", facts.EIN)
	assert.Equal(t, "TEST", facts.Ticker)
	assert.Equal(t, "Test Company", facts.CompanyName)
	assert.Equal(t, 100, facts.EmployeesCount)
	assert.Equal(t, 1000000, facts.TotalRevenue)
	assert.Equal(t, 800000, facts.TotalExpenses)
}