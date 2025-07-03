package facts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/saranrapjs/labor-leverage/pkg/edgar"
)

func TestExtractCEOPayRatio(t *testing.T) {
	tests := []struct {
		name             string
		filename         string
		expectedCEOPay   float64
		expectedMedianPay float64
	}{
		{
			name:             "example fixture",
			filename:         "apple.html",
			expectedCEOPay:   74609802,
			expectedMedianPay: 114738,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Load HTML file from fixtures
			fixturesDir := filepath.Join(".", "fixtures")
			htmlPath := filepath.Join(fixturesDir, tt.filename)
			
			htmlContent, err := os.ReadFile(htmlPath)
			require.NoError(t, err, "Failed to read fixture file: %s", tt.filename)

			// Create mock Edgar document
			doc := edgar.Document{
				DocumentFile: htmlContent,
				Filing:       edgar.Filing{},
			}

			// Extract facts
			facts, err := ExtractFacts("test-cik", "TEST", "Test Company", []edgar.Document{doc})
			require.NoError(t, err, "Failed to extract facts")

			// Verify CEO pay ratio
			if tt.expectedCEOPay > 0 || tt.expectedMedianPay > 0 {
				require.NotNil(t, facts.CEOPayRatio, "Expected CEOPayRatio to be extracted")
				assert.Equal(t, tt.expectedCEOPay, facts.CEOPayRatio.CEO, "CEO pay mismatch")
				assert.Equal(t, tt.expectedMedianPay, facts.CEOPayRatio.Median, "Median pay mismatch")
			}
		})
	}
}