// Packkage ixbrl implements for parsing and post-processing the
// iXBRL (Inline eXtensible Business Reporting Language) XML grammar
// embedded in HTML files published by businesses to the SEC's EDGAR
// system.
package ixbrl

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
)

type Match struct {
	Node *html.Node
	Text string
}

// SearchHTML searches for HTML nodes whose text content matches the given regular expression.
// It recursively traverses the HTML tree starting from the given node and returns all nodes
// that contain text matching the regex pattern.
func SearchHTML(node *html.Node, predicate func(text string) string) []Match {
	var matches []Match
	searchHTMLRecursive(node, predicate, &matches)
	return matches
}

func searchHTMLRecursive(node *html.Node, predicate func(text string) string, matches *[]Match) {
	if node == nil {
		return
	}

	// Check if this is a leaf node (no element children)
	if isLeafNode(node) {
		// Get the text content of the current node
		textContent := HTMLText(node)

		// Check if the text content matches the regex
		if textContent != "" {
			if match := predicate(textContent); match != "" {
				*matches = append(*matches, Match{
					Node: node,
					Text: match,
				})
			}
		}
		return
	}

	// Recursively search child nodes
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		searchHTMLRecursive(child, predicate, matches)
	}
}

// HTMLText uses an HTML stringification algorithm geared towards reproducing
// the way the an HTML node's text would display in a browser:
// - block nodes are wrapped with line returns
// - inline nodes have contiguous spaces collapsed down to one space
// - non-text nodes (e.g. HTML comments or text nodes between block nodes) are omitted
func HTMLText(nodes ...*html.Node) string {
	if nodes == nil {
		return ""
	}

	var textBuilder strings.Builder
	for _, node := range nodes {
		extractText(node, &textBuilder)
	}
	return strings.TrimSpace(textBuilder.String())
}

func isInlineNode(node *html.Node) bool {
	if node.Type != html.ElementNode {
		return true
	}
	switch node.Data {
	case "span", "em", "strong", "a", "br":
		return true
	default:
		return false
	}
}

// extractText recursively extracts text content from a node and its descendants
func extractText(node *html.Node, builder *strings.Builder) {
	if node == nil {
		return
	}

	if node.Type == html.TextNode {
		text := node.Data
		if text != "" {
			builder.WriteString(text)
		}
	}
	var cb strings.Builder
	allInlineChildren := onlyInlineChildren(node)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if !allInlineChildren && child.Type == html.TextNode {
			continue
		}
		extractText(child, &cb)
	}
	if allInlineChildren {
		builder.WriteString(strings.Join(strings.Fields(cb.String())," "))
	} else {
		builder.WriteString(cb.String())
	}
	if node.Type == html.ElementNode && !isInlineNode(node) {
		builder.WriteString("\n")
	}
}

// isLeafNode checks if a node is a leaf node (has no element children)
func isLeafNode(node *html.Node) bool {
	if node == nil {
		return false
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			return false
		}
	}
	return true
}

func onlyInlineChildren(node *html.Node) bool {
	allInlineChildren := true
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if !isInlineNode(child) {
			allInlineChildren = false
			break
		}
	}
	return allInlineChildren
}

// FindTables searches for table elements whose text content matches the given predicate.
// It recursively traverses the HTML tree starting from the given node and returns all
// table nodes that contain text matching the predicate function.
func FindTables(node *html.Node, predicate func(text string) bool) []*html.Node {
	var tables []*html.Node
	findTablesRecursive(node, predicate, &tables)
	return tables
}

func findTablesRecursive(node *html.Node, predicate func(text string) bool, tables *[]*html.Node) {
	if node == nil {
		return
	}

	// Check if this is a table element
	if node.Type == html.ElementNode && node.Data == "table" {
		// Get the text content of the table
		textContent := HTMLText(node)
		
		// Check if the text content matches the predicate
		if textContent != "" && predicate(textContent) {
			*tables = append(*tables, node)
		}
	}

	// Recursively search child nodes
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		findTablesRecursive(child, predicate, tables)
	}
}

type leafAccumulator struct {
	nodes []*html.Node
}

func (l *leafAccumulator) String() string {
	return HTMLText(l.nodes...)
}

func (l *leafAccumulator) Len() int {
	return len(l.String())
}

func (l *leafAccumulator) Add(n *html.Node) {
	if slices.Contains(l.nodes, n) {
		return
	}
	l.nodes = append(l.nodes, n)
}

// FindNextLeafNodes finds nearby leaf nodes at a similar depth as the given node.
// It performs a depth-first traversal starting from the node's next sibling, then moves up
// to parent levels to continue the search if needed, until HTML nodes with n number of
// characters have been found, returning the stringified text using [HTMLText].
func FindNextLeafNodes(node *html.Node, n int) string {
	if node == nil || n <= 0 {
		return ""
	}

	var result leafAccumulator
	targetDepth := getNodeDepth(node) - 2
	
	// Start search from the next sibling
	current := node.NextSibling
	parent := node.Parent
	
	for result.Len() < n && (current != nil || parent != nil) {
		if current != nil {
			// Search in current subtree
			findTextNodesAtDepth(current, targetDepth, &result, n)
			current = current.NextSibling
		} else if parent != nil {
			// Move up one level and continue with parent's next sibling
			current = parent.NextSibling
			parent = parent.Parent
		}
	}
	return result.String()
}

func getNodeDepth(node *html.Node) int {
	depth := 0
	for p := node.Parent; p != nil; p = p.Parent {
		depth++
	}
	return depth
}

func findTextNodesAtDepth(node *html.Node, targetDepth int, result *leafAccumulator, maxLength int) {
	if node == nil || result.Len() >= maxLength {
		return
	}
	
	currentDepth := getNodeDepth(node)
	
	// If we're at or below target depth and this is a leaf node with some text, add it
	if currentDepth >= targetDepth && isLeafNode(node) {
		result.Add(node)
		return
	}
	
	// Continue searching children if we haven't reached max nodes
	for child := node.FirstChild; child != nil && result.Len() < maxLength; child = child.NextSibling {
		findTextNodesAtDepth(child, targetDepth, result, maxLength)
	}
}

// Print is a debugging aid, returning the HTML string for a given *html.Node.
func Print(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	html.Render(&b, n)
	return b.String()
}
