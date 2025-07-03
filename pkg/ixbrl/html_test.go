package ixbrl

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/html"
)

const htmlContent = `<div>
	<p>First paragraph</p>
	<div>
		<span>Nested <br />span</span>
		<a>Link text</a>
	</div>
	<p>Second <span>paragraph</span></p>
	<ul>
		<li>Item 1</li>
		<li>Item 2</li>
	</ul>
</div>`

func TestText(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}
	text := HTMLText(doc)
	expected := `First paragraph
Nested span Link text
Second paragraph
Item 1
Item 2`
	if text != expected {
		t.Errorf("expected:\n%v\n got:\n%v", expected, text)
	}
}

func TestFindNextLeafNodes(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	// Find the first paragraph node
	res := SearchHTML(doc, func(str string) string {
		if str == "First paragraph" {
			return str			
		}
		return ""
	})

	if len(res) == 0 || res[0].Node == nil {
		t.Fatal("Could not find first paragraph node")
	}
	firstP := res[0].Node

	// Test finding next 3 leaf nodes
	text := FindNextLeafNodes(firstP, 30)
	expectedText := "Nested span\n\t\tLink text\n\t\n\tSecond"
	assert.Equal(t, expectedText, text)
}

func TestWeird(t *testing.T) {
	const d = `<div style="width:97%; margin-top:1.5%; margin-bottom:1.5%; margin-left:1.5%; margin-right:-1.25%">
 <p style="margin-top:0pt; margin-bottom:0pt; font-size:12pt; font-family:arial;font-weight:bold" id="toc858775_23a">Payments at Termination of Employment </p> <p style="margin-top:6pt; margin-bottom:0pt; font-size:9pt; font-family:arial">This table shows the amounts that would have been payable to our Named Executives upon a termination of employment without cause or due to retirement on June&nbsp;30, 2024, or at a termination due to death or disability on June&nbsp;30, 2024, pursuant to our post-employment compensation arrangements as described on pages 53-54. The equity values presented in the table below were valued using the closing stock price on June&nbsp;28, 2024, which was $446.95 per share. </p> <p style="font-size:12pt;margin-top:0pt;margin-bottom:0pt">&nbsp;</p>
<table cellspacing="0" cellpadding="0" style="BORDER-COLLAPSE:COLLAPSE; font-family:arial; font-size:9pt;width:100%;border:0">


<tbody><tr>

<td style="width:89%">

</td><td style="vertical-align:bottom;width:2%">
</td><td>
</td><td>
</td><td>

</td><td style="vertical-align:bottom;width:4%">
</td><td>

</td><td style="vertical-align:bottom;width:4%">
</td><td></td></tr>
<tr style="page-break-inside:avoid ; font-family:arial; font-size:9pt">
<td style="padding-bottom:2pt ;BORDER-BOTTOM:1.00pt solid #000000;vertical-align:bottom"> <p style=" margin-top:0pt ; margin-bottom:0pt; text-indent:0.44em; font-size:9pt; font-family:arial;font-weight:bold">Named Executive</p></td>
<td style=" BORDER-BOTTOM:1.00pt solid #000000;vertical-align:bottom">&nbsp;&nbsp;</td>
<td colspan="2" align="right" style="padding-bottom:2pt ;BORDER-BOTTOM:1.00pt solid #000000;vertical-align:bottom"> <p style="margin-top:0pt; margin-bottom:0pt; font-size:9pt; font-family:arial;font-weight:bold;text-align:right">Without&nbsp;Cause<sup style="font-size:75%; vertical-align:top">1</sup></p> <p style="margin-top:0pt; margin-bottom:1pt; font-size:9pt; font-family:arial;font-weight:bold;text-align:right">($)</p></td>
<td style=" BORDER-BOTTOM:1.00pt solid #000000;vertical-align:bottom">&nbsp;</td>
<td style=" BORDER-BOTTOM:1.00pt solid #000000;vertical-align:bottom">&nbsp;&nbsp;&nbsp;&nbsp;</td>
<td align="right" style="padding-bottom:2pt ;BORDER-BOTTOM:1.00pt solid #000000;vertical-align:bottom"> <p style="margin-top:0pt; margin-bottom:0pt; font-size:9pt; font-family:arial;font-weight:bold;text-align:right">Retirement</p> <p style="margin-top:0pt; margin-bottom:1pt; font-size:9pt; font-family:arial;font-weight:bold;text-align:right">($)</p></td>
<td style=" BORDER-BOTTOM:1.00pt solid #000000;vertical-align:bottom">&nbsp;&nbsp;&nbsp;&nbsp;</td>
<td align="right" style="padding-bottom:2pt ;BORDER-BOTTOM:1.00pt solid #000000;vertical-align:bottom"> <p style="margin-top:0pt; margin-bottom:0pt; font-size:9pt; font-family:arial;font-weight:bold;text-align:right">Death&nbsp;or&nbsp;Disability</p> <p style="margin-top:0pt; margin-bottom:1pt; font-size:9pt; font-family:arial;font-weight:bold;text-align:right">($)</p></td></tr>


<tr style="font-size:1pt">
<td style="height:3.75pt">
</td><td style="height:3.75pt" colspan="4">
</td><td style="height:3.75pt" colspan="2">
</td><td style="height:3.75pt" colspan="2"></td></tr>
<tr style="page-break-inside:avoid ; font-family:arial; font-size:9pt">
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:top"> <p style=" margin-top:0pt ; margin-bottom:0pt; margin-left:1.44em; text-indent:-1.00em; font-size:9pt; font-family:arial"><span style="color:#0075c9"><span style="font-weight:bold">Satya Nadella</span></span></p></td>
<td style=" BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;&nbsp;</td>
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;</td>
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom" align="right">173,145,515</td>
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;white-space:nowrap;vertical-align:bottom">&nbsp;</td>
<td style=" BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;&nbsp;&nbsp;&nbsp;</td>
<td align="right" style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">132,865,273</td>
<td style=" BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;&nbsp;&nbsp;&nbsp;</td>
<td align="right" style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">229,537,430</td></tr>
<tr style="font-size:1pt">
<td style="height:3.75pt">
</td><td style="height:3.75pt" colspan="4">
</td><td style="height:3.75pt" colspan="2">
</td><td style="height:3.75pt" colspan="2"></td></tr>
<tr style="page-break-inside:avoid ; font-family:arial; font-size:9pt">
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:top"> <p style=" margin-top:0pt ; margin-bottom:0pt; margin-left:1.44em; text-indent:-1.00em; font-size:9pt; font-family:arial"><span style="color:#0075c9"><span style="font-weight:bold">Amy E. Hood</span></span></p></td>
<td style=" BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;&nbsp;</td>
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;</td>
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom" align="right">46,205,035</td>
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;white-space:nowrap;vertical-align:bottom">&nbsp;</td>
<td style=" BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;&nbsp;&nbsp;&nbsp;</td>
<td align="right" style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">0</td>
<td style=" BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;&nbsp;&nbsp;&nbsp;</td>
<td align="right" style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">67,779,968</td></tr>
<tr style="font-size:1pt">
<td style="height:3.75pt">
</td><td style="height:3.75pt" colspan="4">
</td><td style="height:3.75pt" colspan="2">
</td><td style="height:3.75pt" colspan="2"></td></tr>
<tr style="page-break-inside:avoid ; font-family:arial; font-size:9pt">
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:top"> <p style=" margin-top:0pt ; margin-bottom:0pt; margin-left:1.44em; text-indent:-1.00em; font-size:9pt; font-family:arial"><span style="color:#0075c9"><span style="font-weight:bold">Judson B. Althoff</span></span></p></td>
<td style=" BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;&nbsp;</td>
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;</td>
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom" align="right">38,094,245</td>
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;white-space:nowrap;vertical-align:bottom">&nbsp;</td>
<td style=" BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;&nbsp;&nbsp;&nbsp;</td>
<td align="right" style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">0</td>
<td style=" BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;&nbsp;&nbsp;&nbsp;</td>
<td align="right" style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">56,966,459</td></tr>
<tr style="font-size:1pt">
<td style="height:3.75pt">
</td><td style="height:3.75pt" colspan="4">
</td><td style="height:3.75pt" colspan="2">
</td><td style="height:3.75pt" colspan="2"></td></tr>
<tr style="page-break-inside:avoid ; font-family:arial; font-size:9pt">
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:top"> <p style=" margin-top:0pt ; margin-bottom:0pt; margin-left:1.44em; text-indent:-1.00em; font-size:9pt; font-family:arial"><span style="color:#0075c9"><span style="font-weight:bold">Bradford L. Smith</span></span></p></td>
<td style=" BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;&nbsp;</td>
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;</td>
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom" align="right">47,989,259</td>
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;white-space:nowrap;vertical-align:bottom">&nbsp;</td>
<td style=" BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;&nbsp;&nbsp;&nbsp;</td>
<td align="right" style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">34,450,906</td>
<td style=" BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;&nbsp;&nbsp;&nbsp;</td>
<td align="right" style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">60,003,931</td></tr>
<tr style="font-size:1pt">
<td style="height:3.75pt">
</td><td style="height:3.75pt" colspan="4">
</td><td style="height:3.75pt" colspan="2">
</td><td style="height:3.75pt" colspan="2"></td></tr>
<tr style="page-break-inside:avoid ; font-family:arial; font-size:9pt">
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:top"> <p style=" margin-top:0pt ; margin-bottom:0pt; margin-left:1.44em; text-indent:-1.00em; font-size:9pt; font-family:arial"><span style="color:#0075c9"><span style="font-weight:bold">Christopher D. Young</span></span></p></td>
<td style=" BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;&nbsp;</td>
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;</td>
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom" align="right">30,996,923</td>
<td style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;white-space:nowrap;vertical-align:bottom">&nbsp;</td>
<td style=" BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;&nbsp;&nbsp;&nbsp;</td>
<td align="right" style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">0</td>
<td style=" BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">&nbsp;&nbsp;&nbsp;&nbsp;</td>
<td align="right" style="padding-bottom:2pt ;BORDER-BOTTOM:0.75pt solid #0075c9;vertical-align:bottom">38,535,135</td></tr>
</tbody></table> <p style="font-size:4pt;margin-top:0pt;margin-bottom:0pt">&nbsp;</p>
<table style="BORDER-COLLAPSE:COLLAPSE; font-family:arial; font-size:7pt;border:0;width:100%" cellpadding="0" cellspacing="0">
<tbody><tr style="page-break-inside:avoid">
<td style="width:3%;vertical-align:top" align="left">(1)</td>
<td align="left" style="vertical-align:top"> <p style=" margin-top:0pt ; margin-bottom:0pt; font-size:7pt; font-family:arial;text-align:left">Termination without cause includes incremental value associated with retirement-based stock vesting of SAs: $6,403,006 for Mr. Smith. </p></td></tr></tbody></table> <p style="font-size:24pt;margin-top:0pt;margin-bottom:0pt">&nbsp;</p> <p style="line-height:1.0pt;margin-top:0pt;margin-bottom:2pt;border-bottom:1px solid #0075c9">&nbsp;</p> <p style="margin-top:4pt; margin-bottom:0pt; font-size:14pt; font-family:arial" id="toc858775_24"><span style="color:#0075c9"><span style="font-weight:bold">CEO Pay Ratio </span></span></p> <p style="margin-top:6pt; margin-bottom:0pt; font-size:9pt; font-family:arial">For fiscal year 2024, the annual total compensation for the median employee of the Company (other than our CEO) was $193,744 and the annual total compensation of our CEO was $79,106,183. Based on this information, for fiscal year 2024 the ratio of the annual total compensation of our CEO to the annual total compensation of the median employee was 408 to 1. We believe this ratio is a reasonable estimate calculated in a manner consistent with Item&nbsp;402(u) of Regulation <span style="white-space:nowrap">S-K</span> under the Securities Exchange Act of 1934. </p> <p style="margin-top:6pt; margin-bottom:0pt; font-size:9pt; font-family:arial">We identified our median employee from among our employees as of June&nbsp;30, 2024, the last day of our fiscal year, excluding approximately 11,825 employees who became Microsoft employees in fiscal year 2024 as a result of the business acquisition of Activision Blizzard, Inc. To identify our median employee, we used a “total direct compensation” measure consisting of: (i)&nbsp;fiscal year 2024 annual base pay (salary or gross wages for hourly employees, excluding paid leave), which we annualized for any permanent employees who commenced work during the year, (ii)&nbsp;target bonuses and cash incentives payable for fiscal year 2024 (excluding allowances, relocation payments, and profit-sharing), and (iii)&nbsp;the dollar value of SAs and target PSAs granted in fiscal year 2024. Compensation amounts were determined from our human resources and payroll systems of record. Payments not made in U.S. dollars were converted to U.S. dollars using <span style="white-space:nowrap">12-month</span> average exchange rates for the year. To identify our median employee, we then calculated the total direct compensation for our global employee population and excluded employees at the median who had anomalous compensation characteristics. </p>
 <p style="margin-top:0pt;margin-bottom:0pt ; font-size:8pt">&nbsp;</p></div>`
	doc, err := html.Parse(strings.NewReader(d))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}
	res := SearchHTML(doc, func(str string) string {
		if str == "CEO Pay Ratio" {
			return str			
		}
		return ""
	})
	if len(res) == 0 || res[0].Node == nil {
		t.Fatal("Could not find first paragraph node")
	}
	firstP := res[0].Node
	text := FindNextLeafNodes(firstP, 300)
	expectedText := "For fiscal year 2024, the annual total compensation for the median employee of the Company (other than our CEO) was $193,744 and the annual total compensation of our CEO was $79,106,183. Based on this information, for fiscal year 2024 the ratio of the annual total compensation of our CEO to the annual total compensation of the median employee was 408 to 1. We believe this ratio is a reasonable estimate calculated in a manner consistent with Item 402(u) of Regulation"
	if text != expectedText {
		t.Errorf("Expected leaf node to contain:\n%s\ngot:\n%s", expectedText, text)
	}
}
