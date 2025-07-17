package irsform

import (
	"encoding/xml"
	"fmt"
	"io"
	"slices"
)

// SupportedReturnTypes contains the return types that can be unmarshalled by this package
var SupportedReturnTypes = []string{"990", "990EZ"}

// IsSupportedReturnType checks if a return type is supported for parsing
func IsSupportedReturnType(returnType string) bool {
	return slices.Contains(SupportedReturnTypes, returnType)
}

// ReturnDataInterface represents the interface that all return data types must implement
type ReturnDataInterface interface {
	GetFormType() string
}

// Filer represents the filer information in the return header
type Filer struct {
	BusinessName BusinessNameType `xml:"BusinessName"`
}

// ReturnHeader represents the header section of an IRS return
type ReturnHeader struct {
	ReturnTypeCd     string `xml:"ReturnTypeCd"`
	Filer            Filer  `xml:"Filer"`
	TaxPeriodEndDt   string `xml:"TaxPeriodEndDt"`
	TaxPeriodBeginDt string `xml:"TaxPeriodBeginDt"`
}

// Return is an IRS Return - wraps around Return Header and Return Data.
// Used for forms 990, 990EZ and 990PF.
type Return struct {
	XMLName           xml.Name            `xml:"Return"`
	ReturnVersionAttr string              `xml:"returnVersion,attr"`
	ReturnHeader      ReturnHeader        `xml:"ReturnHeader"`
	ReturnData        ReturnDataInterface `xml:"-"`
}

// Parse parses an XML document and returns a Return struct
func Parse(r io.Reader) (*Return, error) {
	// Read all data first so we can parse it twice
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	// First pass: Parse just to get the ReturnHeader and determine the form type
	type HeaderOnlyReturn struct {
		XMLName           xml.Name     `xml:"Return"`
		ReturnVersionAttr string       `xml:"returnVersion,attr"`
		ReturnHeader      ReturnHeader `xml:"ReturnHeader"`
	}

	var headerReturn HeaderOnlyReturn
	if err := xml.Unmarshal(data, &headerReturn); err != nil {
		return nil, fmt.Errorf("failed to parse return header: %w", err)
	}

	if headerReturn.ReturnHeader.ReturnTypeCd == "" {
		return nil, fmt.Errorf("ReturnTypeCd is empty in header")
	}

	// Second pass: Parse with the correct ReturnData type based on ReturnTypeCd
	switch headerReturn.ReturnHeader.ReturnTypeCd {
	case "990":
		type Return990 struct {
			XMLName           xml.Name      `xml:"Return"`
			ReturnVersionAttr string        `xml:"returnVersion,attr"`
			ReturnHeader      ReturnHeader  `xml:"ReturnHeader"`
			ReturnData        ReturnData990 `xml:"ReturnData"`
		}
		var ret990 Return990
		if err := xml.Unmarshal(data, &ret990); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Return with ReturnData990: %w", err)
		}
		return &Return{
			XMLName:           ret990.XMLName,
			ReturnVersionAttr: ret990.ReturnVersionAttr,
			ReturnHeader:      ret990.ReturnHeader,
			ReturnData:        &ret990.ReturnData,
		}, nil

	case "990EZ":
		type Return990EZ struct {
			XMLName           xml.Name        `xml:"Return"`
			ReturnVersionAttr string          `xml:"returnVersion,attr"`
			ReturnHeader      ReturnHeader    `xml:"ReturnHeader"`
			ReturnData        ReturnData990EZ `xml:"ReturnData"`
		}
		var ret990EZ Return990EZ
		if err := xml.Unmarshal(data, &ret990EZ); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Return with ReturnData990EZ: %w", err)
		}
		return &Return{
			XMLName:           ret990EZ.XMLName,
			ReturnVersionAttr: ret990EZ.ReturnVersionAttr,
			ReturnHeader:      ret990EZ.ReturnHeader,
			ReturnData:        &ret990EZ.ReturnData,
		}, nil

	case "990PF":
		type Return990PF struct {
			XMLName           xml.Name        `xml:"Return"`
			ReturnVersionAttr string          `xml:"returnVersion,attr"`
			ReturnHeader      ReturnHeader    `xml:"ReturnHeader"`
			ReturnData        ReturnData990PF `xml:"ReturnData"`
		}
		var ret990PF Return990PF
		if err := xml.Unmarshal(data, &ret990PF); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Return with ReturnData990PF: %w", err)
		}
		return &Return{
			XMLName:           ret990PF.XMLName,
			ReturnVersionAttr: ret990PF.ReturnVersionAttr,
			ReturnHeader:      ret990PF.ReturnHeader,
			ReturnData:        &ret990PF.ReturnData,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported return type: '%s'", headerReturn.ReturnHeader.ReturnTypeCd)
	}
}
