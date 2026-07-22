package resources

import (
	"encoding/json"
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// CloudHSMInstance handles `tencentcloud_cloudhsm_instance` (Cloud HSM / 云加密机
// VSM).
//
// Pricing API (cloudhsm): InquiryPriceBuyVsm (Inquiry, with 'y').
// Docs: https://cloud.tencent.com/document/api/1300/60498
//
// The request takes GoodsNum/PayMode/TimeSpan/TimeUnit. cloudhsm supports a
// prepaid mode (PayMode=1) billed by month; cloudtab always prices a single
// month (TimeSpan=1, TimeUnit="m") so the returned total is the monthly
// run-rate. Response.{OriginalCost,TotalCost} are float64 in 元 (TotalCost is
// the discounted/payable amount, OriginalCost the list price). Currency is
// echoed from the request (default CNY).
type CloudHSMInstance struct{}

func (CloudHSMInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	count := getInt(r.After, "goods_num")
	if count <= 0 {
		count = 1
	}

	params := map[string]interface{}{
		"GoodsNum": count,
		"PayMode":  1, // 1 = prepaid (monthly)
		// Always price a single month; cloudtab reports a monthly run-rate.
		"TimeSpan": "1",
		"TimeUnit": "m",
		"Currency": "CNY",
		"Type":     "CREATE",
	}

	return pricing.PriceRequest{
		Product: "cloudhsm",
		Action:  "InquiryPriceBuyVsm",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (CloudHSMInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	type priceBlock struct {
		// TotalCost is the payable (discounted) amount; OriginalCost the list
		// price. Both are 元. Pointers so we can distinguish absent from zero.
		TotalCost    *float64 `json:"TotalCost"`
		OriginalCost *float64 `json:"OriginalCost"`
	}
	var wrap struct {
		priceBlock
		Response struct {
			priceBlock
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	pb := wrap.priceBlock
	if wrap.Response.TotalCost != nil || wrap.Response.OriginalCost != nil {
		pb = wrap.Response.priceBlock
	}

	// Prefer the payable TotalCost; fall back to OriginalCost. Values are 元/month.
	monthly := 0.0
	if pb.TotalCost != nil && *pb.TotalCost > 0 {
		monthly = *pb.TotalCost
	} else if pb.OriginalCost != nil {
		monthly = *pb.OriginalCost
	}

	return []output.CostComponent{{
		Name:        fmt.Sprintf("Cloud HSM (x%v)", req.Params["GoodsNum"]),
		Unit:        "MONTH",
		HourlyCost:  0,
		MonthlyCost: monthly,
		Currency:    "CNY",
	}}, nil
}
