package irsform

import (
	_ "embed"
	"strings"
	"testing"
)

//go:embed testdata/990.xml
var testXML string

func TestParse(t *testing.T) {
	reader := strings.NewReader(testXML)
	result, err := Parse(reader)
	
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	
	if result == nil {
		t.Fatal("Parse returned nil result")
	}
	
	// Basic validation that we got a Return struct
	if result.ReturnVersionAttr == "" {
		t.Error("Expected ReturnVersionAttr to be set")
	}
	
	if result.ReturnHeader.ReturnTypeCd == "" {
		t.Error("Expected ReturnHeader.ReturnTypeCd to be set")
	}
	
	if result.ReturnData == nil {
		t.Error("Expected ReturnData to contain data")
		t.Logf("ReturnHeader.ReturnTypeCd: %s", result.ReturnHeader.ReturnTypeCd)
		return // Don't continue if ReturnData is nil to avoid panic
	}
	
	// Test the new interface functionality
	formType := result.ReturnData.GetFormType()
	if formType != "990" {
		t.Errorf("Expected form type '990', got '%s'", formType)
	}
	
	// Verify it's actually a ReturnData990 type
	returnData990, ok := result.ReturnData.(*ReturnData990)
	if !ok {
		t.Errorf("Expected ReturnData to be *ReturnData990, got %T", result.ReturnData)
	} else {
		// Verify the IRS990 field is properly populated
		if returnData990.IRS990 == nil {
			t.Error("Expected IRS990 field to be populated")
		}
		t.Logf("Successfully parsed IRS990 form data")
	}
	
	t.Logf("Successfully parsed return with version: %s, header return type: %s, and form type: %s", result.ReturnVersionAttr, result.ReturnHeader.ReturnTypeCd, formType)
}
