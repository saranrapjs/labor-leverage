package ixbrl

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type nodeRegistry map[string]func() interface{}

var registry = nodeRegistry{
	"ix:nonfraction": func() interface{} { return &NonFraction{} },
	"ix:nonnumeric":  func() interface{} { return &NonNumeric{} },
	"ix:fraction":    func() interface{} { return &Fraction{} },
	"xbrli:context":  func() interface{} { return &Context{} },
	"xbrli:unit":     func() interface{} { return &Unit{} },
}

// ParsedNode represents a parsed namespaced node with its unmarshalled struct
type ParsedNode struct {
	Node   *html.Node
	Struct interface{}
	Type   string
}

// Parse parses an XHTML document and returns parsed iXBRL nodes,
// alongside the parsed XHTML document.
func Parse(r io.Reader) ([]*ParsedNode, *html.Node, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, doc, err
	}

	var parsedNodes []*ParsedNode
	collectAndParseNodes(doc, &parsedNodes)
	contexts := getContexts(parsedNodes)
	for i, p := range parsedNodes {
		node := p.Struct
		switch n := node.(type) {
		case *NonFraction:
			{
				for _, c := range contexts {
					if c.ID == n.ContextRef {
						n.Context = c
						p.Struct = n
						parsedNodes[i] = p
					}
				}
			}
		case *NonNumeric:
			{
				for _, c := range contexts {
					if c.ID == n.ContextRef {
						n.Context = c
						p.Struct = n
						parsedNodes[i] = p
					}
				}
			}
		case *Fraction:
			{
				for _, c := range contexts {
					if c.ID == n.ContextRef {
						n.Context = c
						p.Struct = n
						parsedNodes[i] = p
					}
				}
			}
		}
	}
	return parsedNodes, doc, nil
}


// collectAndParseNodes recursively traverses the HTML tree and
// collects/parses nodes with colons, as a fuzzy test for
// whether or not they are likely to correspond to iXBRL tags.
func collectAndParseNodes(n *html.Node, nodes *[]*ParsedNode) {
	if n.Type == html.ElementNode && strings.Contains(n.Data, ":") {
		parsedNode := &ParsedNode{
			Node: n,
			Type: n.Data,
		}

		// Try to unmarshal into a registered struct type
		if constructor, exists := registry[n.Data]; exists {
			structInstance := constructor()
			var s strings.Builder
			if err := html.Render(&s, n); err != nil {
				fmt.Printf("error re-serializing xml: %v\n", err)
				return
			}
			if err := xml.Unmarshal([]byte(s.String()), structInstance); err != nil {
				fmt.Printf("error conforming xml: %v\n", err)
				return
			}
			parsedNode.Struct = structInstance
		}

		*nodes = append(*nodes, parsedNode)
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectAndParseNodes(c, nodes)
	}
}

// NonFraction represents ix:nonfraction elements. These are numeric facts that are not fractions,
// typically used for financial data that can be scaled (thousands, millions, etc.).
type NonFraction struct {
	XMLName    xml.Name `xml:"nonfraction"`
	UnitRef    string   `xml:"unitref,attr"`
	Decimals   string   `xml:"decimals,attr"`
	Name       string   `xml:"name,attr"`
	Format     string   `xml:"format,attr"`
	Scale      string   `xml:"scale,attr"`
	ID         string   `xml:"id,attr"`
	Content    string   `xml:",chardata"`
	ContextRef string   `xml:"contextref,attr"`
	Context    *Context
}

func (nf *NonFraction) scale() float64 {
	scale, err := strconv.Atoi(nf.Scale)
	if err != nil {
		return 1
	}
	return math.Pow10(scale)
}


func (nf *NonFraction) number() float64 {
	// Remove commas and parse
	cleanContent := strings.ReplaceAll(nf.Content, ",", "")
	value, err := strconv.ParseFloat(cleanContent, 64)
	if err != nil {
		return 0
	}
	return value
}

// Applies the scale factor (stored as a power of 10)
// to the non-fractional value.
func (nf *NonFraction) ScaledNumber() float64 {
	return nf.scale() * nf.number();
}

// NonNumeric represents ix:nonnumeric elements. These are textual or non-numeric facts,
// such as company names, descriptions, or other qualitative information.
type NonNumeric struct {
	XMLName    xml.Name `xml:"nonnumeric"`
	Name       string   `xml:"name,attr"`
	Format     string   `xml:"format,attr"`
	ID         string   `xml:"id,attr"`
	Content    string   `xml:",chardata"`
	ContextRef string   `xml:"contextref,attr"`
	Context    *Context
}

// Fraction represents ix:fraction elements. These are numeric facts reported as fractions
// with separate numerator and denominator components (e.g., 22.5/77.5).
type Fraction struct {
	XMLName    xml.Name `xml:"fraction"`
	UnitRef    string   `xml:"unitref,attr"`
	Name       string   `xml:"name,attr"`
	ID         string   `xml:"id,attr"`
	Content    string   `xml:",chardata"`
	ContextRef string   `xml:"contextref,attr"`
	Context    *Context
}

// Context represents xbrli:context elements. These provide dimensional context for facts,
// including entity identification, time period, and segment information.
type Context struct {
	XMLName xml.Name `xml:"context"`
	ID      string   `xml:"id,attr"`
	Entity  Entity   `xml:"entity"`
	Period  Period   `xml:"period"`
}

// Period represents xbrli:period elements within contexts. These define the time period for a fact,
// either as an instant in time or a duration with start and end dates.
type Period struct {
	XMLName   xml.Name `xml:"period"`
	StartDate string   `xml:"startdate"`
	Instant   string   `xml:"instant"`
	EndDate   string   `xml:"enddate"`
}

func (p Period) FormattedValue() string {
	if p.Instant != "" {
		return p.Instant
	}
	return fmt.Sprintf("%s thru %s", p.StartDate, p.EndDate)
}

// Entity represents xbrli:entity elements. These identify the reporting entity
// with unique identifier and optional dimensional segments.
type Entity struct {
	Identifier Identifier `xml:"identifier"`
	Segment    Segment    `xml:"segment"`
}

// Identifier represents xbrli:identifier elements. These provide unique identifier for an entity
// using a specific identification scheme (e.g., SEC CIK, LEI).
type Identifier struct {
	XMLName xml.Name `xml:"identifier"`
	Scheme  string   `xml:"scheme,attr"`
	Content string   `xml:",chardata"`
}

// Segment represents xbrli:segment elements. These provide dimensional breakdown of entity context
// containing explicit and typed members for detailed categorization.
type Segment struct {
	XMLName         xml.Name         `xml:"segment"`
	ExplicitMembers []ExplicitMember `xml:"explicitmember"`
	TypedMembers    []TypedMember    `xml:"typedmember"`
}

// ExplicitMember represents xbrldi:explicitMember elements. These define explicit dimensional members
// that specify categories or breakdowns within a segment.
type ExplicitMember struct {
	XMLName   xml.Name `xml:"explicitmember"`
	Dimension string   `xml:"dimension,attr"`
	Content   string   `xml:",chardata"`
}

// TypedMember represents xbrldi:typedMember elements. These provide typed dimensional members
// that enable flexible dimensional categorization using custom data types.
type TypedMember struct {
	XMLName   xml.Name `xml:"typedmember"`
	Dimension string   `xml:"dimension,attr"`
	Content   string   `xml:",chardata"`
}

// Unit represents xbrli:unit elements. These define the unit of measurement for numeric facts
// (e.g., USD, shares, square feet, percentages).
type Unit struct {
	XMLName xml.Name `xml:"unit"`
	ID      string   `xml:"id,attr"`
	Measure Measure  `xml:"measure"`
}

// Measure represents xbrli:measure elements. These specify the actual measurement unit
// using standardized unit identifiers or custom measures.
type Measure struct {
	XMLName xml.Name `xml:"measure"`
	Content string   `xml:",chardata"`
}

// FilterByType returns all parsed nodes of a specific iXBRL type.
func FilterByType[K interface{}](nodes []*ParsedNode, predicate func(t *K) bool) []*K {
	var filtered []*K
	for _, node := range nodes {
		if t, ok := node.Struct.(*K); ok {
			if predicate(t) {
				filtered = append(filtered, t)
			}
		}
	}
	return filtered
}

func getContexts(nodes []*ParsedNode) []*Context {
	var contexts []*Context
	for _, node := range nodes {
		if ctx, ok := node.Struct.(*Context); ok {
			contexts = append(contexts, ctx)
		}
	}
	return contexts
}

// Search for a particular iXBRL node, amongst a set a previously
// pasted nodes, matching a predicate.
func Search[K any](nodes []*ParsedNode, predicate func(val *K) bool) *K {
	for _, node := range nodes {
		if t, ok := node.Struct.(*K); ok {
			if predicate(t) {
				return t
			}
		}
	}
	return nil
}
