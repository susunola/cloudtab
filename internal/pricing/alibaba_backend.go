// Package pricing — Alibaba Cloud BSS pricing backend.
//
// Alibaba Cloud exposes pricing through the BSS OpenAPI GetPayAsYouGoPrice
// endpoint, a unified API that prices all pay-as-you-go products (ECS, RDS,
// Redis, SLB, etc.) via product-specific ModuleList parameters. This backend
// handles the single API call; each mapper (in internal/resources/alibaba_*.go)
// builds the appropriate ModuleList and parses the response.
//
// Contract with Alibaba mappers:
//   - req.Product  = the BSS ProductCode (e.g. "ecs", "rds", "slb").
//   - req.Region   = the Alibaba Cloud region (e.g. "cn-hangzhou").
//   - req.Params   = neutral params map. The backend reads:
//       "SubscriptionType": "PayAsYouGo" | "Subscription" (default: "PayAsYouGo")
//       "ModuleList":        []map[string]interface{} with keys ModuleCode, PriceType, Config
package pricing

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/bssopenapi"
)

// alibabaBSSAPI is the subset of the Alibaba BSS client used by the backend.
type alibabaBSSAPI interface {
	GetPayAsYouGoPrice(request *bssopenapi.GetPayAsYouGoPriceRequest) (*bssopenapi.GetPayAsYouGoPriceResponse, error)
}

// alibabaBackend implements backend using the Alibaba Cloud BSS GetPayAsYouGoPrice API.
type alibabaBackend struct {
	client alibabaBSSAPI
}

// newAlibabaBackend builds the Alibaba BSS backend. Credentials are resolved
// from Config.AlibabaAccessKeyID / Config.AlibabaAccessKeySecret, falling back
// to environment variables ALIBABA_ACCESS_KEY_ID / ALIBABA_ACCESS_KEY_SECRET.
func newAlibabaBackend(cfg Config) (backend, error) {
	ak := cfg.AlibabaAccessKeyID
	sk := cfg.AlibabaAccessKeySecret
	if ak == "" {
		ak = os.Getenv("ALIBABA_ACCESS_KEY_ID")
	}
	if sk == "" {
		sk = os.Getenv("ALIBABA_ACCESS_KEY_SECRET")
	}
	if ak == "" || sk == "" {
		return nil, fmt.Errorf("alibaba: missing access key (set ALIBABA_ACCESS_KEY_ID / ALIBABA_ACCESS_KEY_SECRET or Config.AlibabaAccessKeyID / AlibabaAccessKeySecret)")
	}

	client, err := bssopenapi.NewClientWithAccessKey("cn-hangzhou", ak, sk)
	if err != nil {
		return nil, fmt.Errorf("alibaba: create BSS client: %w", err)
	}
	return &alibabaBackend{client: client}, nil
}

// query runs a single GetPayAsYouGoPrice call.
//
// The mapper's Extract() populates req.Params with ModuleList and optional
// SubscriptionType. The backend builds the typed SDK request, executes it,
// and returns the raw response JSON for the mapper's Parse() to decode.
func (b *alibabaBackend) query(req PriceRequest) ([]byte, error) {
	if req.Product == "" {
		return nil, fmt.Errorf("alibaba: PriceRequest.Product (BSS ProductCode) is required")
	}

	in := bssopenapi.CreateGetPayAsYouGoPriceRequest()
	in.ProductCode = req.Product
	if req.Region != "" {
		in.Region = req.Region
	}

	subType, _ := req.Params["SubscriptionType"].(string)
	if subType == "" {
		subType = "PayAsYouGo"
	}
	in.SubscriptionType = subType

	// Convert the mapper's neutral ModuleList into the SDK's typed struct slice.
	if rawModules, ok := req.Params["ModuleList"]; ok {
		switch ml := rawModules.(type) {
		case []interface{}:
			modules := make([]bssopenapi.GetPayAsYouGoPriceModuleList, 0, len(ml))
			for _, rm := range ml {
				if m, ok := rm.(map[string]interface{}); ok {
					mod := bssopenapi.GetPayAsYouGoPriceModuleList{}
					if v, _ := m["ModuleCode"].(string); v != "" {
						mod.ModuleCode = v
					}
					if v, _ := m["PriceType"].(string); v != "" {
						mod.PriceType = v
					}
					if v, _ := m["Config"].(string); v != "" {
						mod.Config = v
					}
					modules = append(modules, mod)
				}
			}
			if len(modules) > 0 {
				in.ModuleList = &modules
			}
		case []map[string]string:
			modules := make([]bssopenapi.GetPayAsYouGoPriceModuleList, 0, len(ml))
			for _, m := range ml {
				mod := bssopenapi.GetPayAsYouGoPriceModuleList{
					ModuleCode: m["ModuleCode"],
					PriceType:  m["PriceType"],
					Config:     m["Config"],
				}
				modules = append(modules, mod)
			}
			if len(modules) > 0 {
				in.ModuleList = &modules
			}
		}
	}

	out, err := b.client.GetPayAsYouGoPrice(in)
	if err != nil {
		return nil, fmt.Errorf("alibaba GetPayAsYouGoPrice %s: %w", req.Product, err)
	}
	resp, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("alibaba: marshal response: %w", err)
	}
	return resp, nil
}
