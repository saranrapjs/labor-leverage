package facts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/saranrapjs/labor-leverage/pkg/irsform"
)

func TestFromIRS(t *testing.T) {
	// Create mock IRS990 data instead of relying on XML parsing
	mockIRS990 := &irsform.IRS990Type{
		PrincipalOfficerNm:            "John Doe",
		TotalEmployeeCnt:              250,
		CYTotalRevenueAmt:             5000000,
		CYTotalExpensesAmt:            4500000,
		NetAssetsOrFundBalancesEOYAmt: 1200000,
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
					BusinessNameLine1Txt: "Test Organization Inc",
				},
			},
		},
		ReturnData: returnData990,
	}

	// Extract facts using FromIRS
	facts, err := FromIRS(returnDoc)
	require.NoError(t, err, "Failed to extract facts from IRS data")

	// Verify basic facts were extracted
	assert.NotNil(t, facts, "Facts should not be nil")
	assert.Equal(t, "Test Organization Inc", facts.CompanyName, "Company name should match filer business name")
	assert.Equal(t, 250, facts.EmployeesCount, "Employee count should match")
	assert.Equal(t, 5000000, facts.TotalRevenue, "Total revenue should match")
	assert.Equal(t, 4500000, facts.TotalExpenses, "Total expenses should match")
	assert.Equal(t, 1200000, facts.NetAssets, "Net assets should match")

	// Log the extracted values for verification
	t.Logf("Company Name: %s", facts.CompanyName)
	t.Logf("Employee Count: %d", facts.EmployeesCount)
	t.Logf("Total Revenue: %d", facts.TotalRevenue)
	t.Logf("Total Expenses: %d", facts.TotalExpenses)
	t.Logf("Net Assets: %d", facts.NetAssets)
}

func TestFromIRSNilInput(t *testing.T) {
	// Test with nil input
	facts, err := FromIRS(nil)
	assert.Nil(t, facts, "Facts should be nil for nil input")
	assert.Error(t, err, "Should return error for nil input")
	assert.Contains(t, err.Error(), "invalid return data", "Error should mention invalid return data")
}

func TestFromIRSMissingIRS990(t *testing.T) {
	// Test with ReturnData990 that has nil IRS990
	returnData := &irsform.ReturnData990{
		IRS990: nil,
	}
	
	// Create Return document with missing IRS990
	returnDoc := &irsform.Return{
		ReturnHeader: irsform.ReturnHeader{
			ReturnTypeCd: "990",
			Filer: irsform.Filer{
				BusinessName: irsform.BusinessNameType{
					BusinessNameLine1Txt: "Test Organization Inc",
				},
			},
		},
		ReturnData: returnData,
	}
	
	facts, err := FromIRS(returnDoc)
	assert.Nil(t, facts, "Facts should be nil for missing IRS990")
	assert.Error(t, err, "Should return error for missing IRS990")
	assert.Contains(t, err.Error(), "missing IRS990", "Error should mention missing IRS990")
}

func TestFromIRS990EZ(t *testing.T) {
	// Create mock IRS990EZ data
	mockIRS990EZ := &irsform.IRS990EZ{
		IRS990EZType: &irsform.IRS990EZType{
			TotalRevenueAmt:               2000000,
			TotalExpensesAmt:              1800000,
			NetAssetsOrFundBalancesEOYAmt: 500000,
		},
	}

	// Create mock ReturnData990EZ
	returnData990EZ := &irsform.ReturnData990EZ{
		IRS990EZ: mockIRS990EZ,
	}

	// Create mock Return document
	returnDoc := &irsform.Return{
		ReturnHeader: irsform.ReturnHeader{
			ReturnTypeCd: "990EZ",
			Filer: irsform.Filer{
				BusinessName: irsform.BusinessNameType{
					BusinessNameLine1Txt: "Small Organization Inc",
				},
			},
		},
		ReturnData: returnData990EZ,
	}

	// Extract facts using FromIRS
	facts, err := FromIRS(returnDoc)
	require.NoError(t, err, "Failed to extract facts from IRS 990EZ data")

	// Verify basic facts were extracted
	assert.NotNil(t, facts, "Facts should not be nil")
	assert.Equal(t, "Small Organization Inc", facts.CompanyName, "Company name should match filer business name")
	assert.Equal(t, 0, facts.EmployeesCount, "Employee count should be 0 for 990EZ (no TotalEmployeeCnt field)")
	assert.Equal(t, 2000000, facts.TotalRevenue, "Total revenue should match")
	assert.Equal(t, 1800000, facts.TotalExpenses, "Total expenses should match")
	assert.Equal(t, 500000, facts.NetAssets, "Net assets should match")

	// Log the extracted values for verification
	t.Logf("Company Name: %s", facts.CompanyName)
	t.Logf("Employee Count: %d", facts.EmployeesCount)
	t.Logf("Total Revenue: %d", facts.TotalRevenue)
	t.Logf("Total Expenses: %d", facts.TotalExpenses)
	t.Logf("Net Assets: %d", facts.NetAssets)
}