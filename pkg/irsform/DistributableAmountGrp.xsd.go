package irsform

// DistributableAmountGrp ...
type DistributableAmountGrp struct {
	Sect4942j3j5FndtnAndFrgnOrgInd string `xml:"Sect4942j3j5FndtnAndFrgnOrgInd,omitempty"`
	MinimumInvestmentReturnAmt     int    `xml:"MinimumInvestmentReturnAmt,omitempty"`
	TaxBasedOnInvestmentIncomeAmt  int    `xml:"TaxBasedOnInvestmentIncomeAmt,omitempty"`
	IncomeTaxAmt                   int    `xml:"IncomeTaxAmt,omitempty"`
	TotalTaxAmt                    int    `xml:"TotalTaxAmt,omitempty"`
	DistributableBeforeAdjAmt      int    `xml:"DistributableBeforeAdjAmt,omitempty"`
	RecoveriesQualfiedDistriAmt    int    `xml:"RecoveriesQualfiedDistriAmt,omitempty"`
	DistributableBeforeDedAmt      int    `xml:"DistributableBeforeDedAmt,omitempty"`
	DeductionFromDistributableAmt  int    `xml:"DeductionFromDistributableAmt,omitempty"`
	DistributableAsAdjustedAmt     int    `xml:"DistributableAsAdjustedAmt,omitempty"`
}
