package ixbrl

import (
	"os"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	// Load the test fixture
	file, err := os.Open("./fixtures/nyt-20241231.htm")
	if err != nil {
		t.Fatalf("Failed to open test fixture: %v", err)
	}
	defer file.Close()

	// Parse the document
	nodes, _, err := Parse(file)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Basic checks
	if len(nodes) == 0 {
		t.Fatal("Expected to find namespaced nodes, but got none")
	}

	t.Logf("Found %d namespaced nodes", len(nodes))

	// Check that all returned nodes have colons in their names
	for i, node := range nodes {
		if !strings.Contains(node.Type, ":") {
			t.Errorf("Node %d (%s) does not contain a colon", i, node.Type)
		}
	}

	// Check for expected XBRL namespaces based on the document
	expectedNamespaces := []string{
		"dei:",
		"us-gaap:",
		"ix:",
		"nyt:",
		"srt:",
		"xbrli:",
	}

	foundNamespaces := make(map[string]bool)
	for _, node := range nodes {
		// for _, a := range node.Node.Attr {
		// 	// us-gaap:StockRepurchasedDuringPeriodShares
		// 	// us-gaap:StockRepurchasedDuringPeriodValue
		// 	// us-gaap:NetIncomeLoss
		// 	if strings.Contains(a.Val, "NetIncomeLoss") {
		// 		fmt.Println(node.Node.Data, node.Node.Attr)
		// 	}
		// }
		for _, ns := range expectedNamespaces {
			if strings.HasPrefix(node.Type, ns) {
				foundNamespaces[ns] = true
				break
			}
		}
	}

	// Verify we found at least some expected namespaces
	if len(foundNamespaces) == 0 {
		t.Error("Did not find any of the expected XBRL namespaces")
	}

	// Log found namespaces for debugging
	for ns := range foundNamespaces {
		t.Logf("Found namespace: %s", ns)
	}

	// Test some specific node types we expect to find
	nodeTypes := make(map[string]int)
	for _, node := range nodes {
		nodeTypes[node.Type]++
	}

	// Log the most common namespaced elements
	t.Logf("Node type distribution:")
	for nodeType, count := range nodeTypes {
		if count > 1 {
			t.Logf("  %s: %d occurrences", nodeType, count)
		}
	}

	contexts := getContexts(nodes)
	t.Logf("Found %d xbrli:context elements", len(contexts))

	// Test that at least some nodes were successfully unmarshalled
	structCount := 0
	for _, node := range nodes {
		if node.Struct != nil {
			structCount++
		}
	}
	t.Logf("Successfully unmarshalled %d out of %d nodes", structCount, len(nodes))
}

func TestParseEmptyReader(t *testing.T) {
	nodes, _, err := Parse(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Parse failed on empty reader: %v", err)
	}

	if len(nodes) != 0 {
		t.Errorf("Expected 0 nodes for empty reader, got %d", len(nodes))
	}
}

func TestParseSimpleHTML(t *testing.T) {
	html := `<html><body><div>test</div><custom:element>value</custom:element></body></html>`
	nodes, _, err := Parse(strings.NewReader(html))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(nodes) != 1 {
		t.Fatalf("Expected 1 namespaced node, got %d", len(nodes))
	}

	if nodes[0].Type != "custom:element" {
		t.Errorf("Expected 'custom:element', got '%s'", nodes[0].Type)
	}
}

func TestParseNonFraction(t *testing.T) {
	html := `<html><body><ix:nonfraction unitref="usd" contextref="c-20" decimals="-3" name="us-gaap:RentalIncomeNonoperating" format="ixt:num-dot-decimal" scale="3" id="f-2166">27,163</ix:nonfraction></body></html>`
	nodes, _, err := Parse(strings.NewReader(html))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(nodes) != 1 {
		t.Fatalf("Expected 1 namespaced node, got %d", len(nodes))
	}

	node := nodes[0]
	if node.Type != "ix:nonfraction" {
		t.Errorf("Expected 'ix:nonfraction', got '%s'", node.Type)
	}

	// Check that struct was unmarshalled
	if node.Struct == nil {
		t.Fatal("Expected struct to be unmarshalled")
	}

	nf, ok := node.Struct.(*NonFraction)
	if !ok {
		t.Fatalf("Expected NonFraction struct, got %T", node.Struct)
	}

	// Check attributes
	if nf.UnitRef != "usd" {
		t.Errorf("Expected UnitRef 'usd', got '%s'", nf.UnitRef)
	}
	if nf.ContextRef != "c-20" {
		t.Errorf("Expected ContextRef 'c-20', got '%s'", nf.ContextRef)
	}
	if nf.Decimals != "-3" {
		t.Errorf("Expected Decimals '-3', got '%s'", nf.Decimals)
	}
	if nf.Name != "us-gaap:RentalIncomeNonoperating" {
		t.Errorf("Expected Name 'us-gaap:RentalIncomeNonoperating', got '%s'", nf.Name)
	}
	if nf.Format != "ixt:num-dot-decimal" {
		t.Errorf("Expected Format 'ixt:num-dot-decimal', got '%s'", nf.Format)
	}
	if nf.Scale != "3" {
		t.Errorf("Expected Scale '3', got '%s'", nf.Scale)
	}
	if nf.ID != "f-2166" {
		t.Errorf("Expected ID 'f-2166', got '%s'", nf.ID)
	}

	// Check content
	expectedContent := "27,163"
	if nf.Content != expectedContent {
		t.Errorf("Expected Content '%s', got '%s'", expectedContent, nf.Content)
	}

	// Test numeric value extraction (scaled)
	value := nf.ScaledNumber()
	expectedValue := 27163000.0 // 27,163 * 1000 (scale=3)
	if value != expectedValue {
		t.Errorf("Expected numeric value %f, got %f", expectedValue, value)
	}
}

func TestParseNonNumeric(t *testing.T) {
	html := `<html><body><ix:nonnumeric contextref="c-1" name="dei:EntityRegistrantName" format="ixt:text-min-length-1" id="f-1">The New York Times Company</ix:nonnumeric></body></html>`
	nodes, _, err := Parse(strings.NewReader(html))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(nodes) != 1 {
		t.Fatalf("Expected 1 namespaced node, got %d", len(nodes))
	}

	node := nodes[0]
	if node.Struct == nil {
		t.Fatal("Expected struct to be unmarshalled")
	}

	nn, ok := node.Struct.(*NonNumeric)
	if !ok {
		t.Fatalf("Expected NonNumeric struct, got %T", node.Struct)
	}

	if nn.ContextRef != "c-1" {
		t.Errorf("Expected ContextRef 'c-1', got '%s'", nn.ContextRef)
	}
	if nn.Name != "dei:EntityRegistrantName" {
		t.Errorf("Expected Name 'dei:EntityRegistrantName', got '%s'", nn.Name)
	}
	if nn.Content != "The New York Times Company" {
		t.Errorf("Expected Content 'The New York Times Company', got '%s'", nn.Content)
	}
}

func TestFilterByType(t *testing.T) {
	html := `<html><body>
		<ix:nonfraction unitref="usd" contextref="c-1">1000</ix:nonfraction>
		<ix:nonnumeric contextref="c-1" name="test">TestValue</ix:nonnumeric>
		<ix:nonfraction unitref="usd" contextref="c-2">2000</ix:nonfraction>
	</body></html>`

	nodes, _, err := Parse(strings.NewReader(html))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	nonFractionNodes := FilterByType(nodes, func(*NonFraction) bool { return true })
	if len(nonFractionNodes) != 2 {
		t.Errorf("Expected 2 ix:nonfraction nodes, got %d", len(nonFractionNodes))
	}

	nonNumericNodes := FilterByType(nodes, func(*NonNumeric) bool { return true })
	if len(nonNumericNodes) != 1 {
		t.Errorf("Expected 1 ix:nonnumeric node, got %d", len(nonNumericNodes))
	}
}

func TestContext(t *testing.T) {
	html := `<html><body>
	<xbrli:context id="c-36">
		<xbrli:entity>
			<xbrli:identifier scheme="http://www.sec.gov/CIK">0000071691</xbrli:identifier>
			<xbrli:segment>
				<xbrldi:explicitMember dimension="us-gaap:StatementEquityComponentsAxis">us-gaap:TreasuryStockCommonMember</xbrldi:explicitMember>
			</xbrli:segment>
		</xbrli:entity>
		<xbrli:period>
			<xbrli:startDate>2021-12-27</xbrli:startDate>
			<xbrli:endDate>2022-12-31</xbrli:endDate>
		</xbrli:period>
	</xbrli:context></body></html>`
	nodes, _, err := Parse(strings.NewReader(html))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	ctx := Search(nodes, func(t *Context) bool { return true })
	if ctx == nil {
		t.Fatalf("got nil for xbrli:context")
	}
	if ctx.ID != "c-36" {
		t.Errorf("expected c-3, got: %v\n", ctx.ID)
	}
	if ctx.Period.StartDate != "2021-12-27" {
		t.Errorf("expected 2025-02-19, got: %v\n", ctx.Period.StartDate)
	}
	if ctx.Entity.Identifier.Content != "0000071691" {
		t.Errorf("expected 0000071691, got: %v\n", ctx.Entity.Identifier.Content)		
	}
	if ctx.Entity.Segment.ExplicitMembers[0].Content != "us-gaap:TreasuryStockCommonMember" {
		t.Errorf("expected us-gaap:TreasuryStockCommonMember, got: %v\n", ctx.Entity.Segment.ExplicitMembers[0].Content)		
	}
}
