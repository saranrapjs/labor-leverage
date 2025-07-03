package ixbrl_test

import (
	"fmt"
	"strings"

	"github.com/saranrapjs/labor-leverage/pkg/ixbrl"
)

func Example() {
	r := strings.NewReader(`<html><body>
		<div style="display:none;"><ix:hidden>
			<xbrli:context id="c-1">
				<xbrli:period>
					<xbrli:startDate>2021-12-27</xbrli:startDate>
					<xbrli:endDate>2022-12-31</xbrli:endDate>
				</xbrli:period>
			</xbrli:context>
		</ix:hidden></div>
		<p>$<ix:nonFraction unitRef="usd" contextRef="c-1" decimals="-3" name="us-gaap:StockRepurchasedDuringPeriodValue" format="ixt:num-dot-decimal" scale="3" id="f-286">105,056</ix:nonFraction> of shares repurchased</p>
	</body></html>`)
	parsed, _, err := ixbrl.Parse(r)
	if err != nil {
		panic(err)
	}
	stockRepurchase := ixbrl.Search(parsed, func(f *ixbrl.NonFraction) bool {
		return f.Name == "us-gaap:StockRepurchasedDuringPeriodValue"
	})
	fmt.Printf("$%0.3f in stock buybacks", stockRepurchase.ScaledNumber())
	// Output: $105056000.000 in stock buybacks
}
