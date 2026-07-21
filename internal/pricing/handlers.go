// Package pricing — product handler registry.
//
// This file is the single place where per-product SDK knowledge lives. To add
// support for a new Tencent Cloud product, register one productHandler here:
//
//  1. Provide a newClient factory (wraps the product's SDK NewClient).
//  2. Map each supported Action name to an invoker that builds the typed
//     request, binds params, calls the SDK, and returns sdkResult(out, err).
//
// The Engine's Query/invoke path is fully generic and never needs editing.
package pricing

import (
	tcCommon "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcProfile "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"

	cbs "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cbs/v20170312"
	cdb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cdb/v20170320"
	clb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/clb/v20180317"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	postgres "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/postgres/v20170312"
	redis "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/redis/v20180412"
)

// clientFactory builds a typed SDK client for a product in a given region.
type clientFactory func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error)

// actionInvoker executes one pricing Action against an already-created client.
// It receives the raw (interface{}) client and the neutral params map, performs
// the type assertion to the concrete SDK client, and returns the response JSON.
type actionInvoker func(client interface{}, params map[string]interface{}) ([]byte, error)

// productHandler bundles everything the engine needs to price a product.
type productHandler struct {
	product   string
	newClient clientFactory
	actions   map[string]actionInvoker
}

// handlers is the product registry consulted by Engine.Query. Adding a product
// here — and nowhere else — makes it fully supported.
var handlers = map[string]productHandler{
	"cvm": {
		product: "cvm",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return cvm.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			"InquiryPriceRunInstances": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := cvm.NewInquiryPriceRunInstancesRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*cvm.Client).InquiryPriceRunInstances(in)
				return sdkResult(out, err)
			},
		},
	},
	"cbs": {
		product: "cbs",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return cbs.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			"InquiryPriceCreateDisks": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := cbs.NewInquiryPriceCreateDisksRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*cbs.Client).InquiryPriceCreateDisks(in)
				return sdkResult(out, err)
			},
		},
	},
	"clb": {
		product: "clb",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return clb.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			"InquiryPriceCreateLoadBalancer": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := clb.NewInquiryPriceCreateLoadBalancerRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*clb.Client).InquiryPriceCreateLoadBalancer(in)
				return sdkResult(out, err)
			},
		},
	},
	"cdb": {
		product: "cdb",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return cdb.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			"DescribeDBPrice": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := cdb.NewDescribeDBPriceRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*cdb.Client).DescribeDBPrice(in)
				return sdkResult(out, err)
			},
		},
	},
	"postgres": {
		product: "postgres",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return postgres.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			"InquiryPriceCreateDBInstances": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := postgres.NewInquiryPriceCreateDBInstancesRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*postgres.Client).InquiryPriceCreateDBInstances(in)
				return sdkResult(out, err)
			},
		},
	},
	"redis": {
		product: "redis",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return redis.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			"InquiryPriceCreateInstance": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := redis.NewInquiryPriceCreateInstanceRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*redis.Client).InquiryPriceCreateInstance(in)
				return sdkResult(out, err)
			},
		},
	},
}

// SupportedProducts returns the product keys currently registered, useful for
// diagnostics and tests. Order is not guaranteed.
func SupportedProducts() []string {
	out := make([]string, 0, len(handlers))
	for p := range handlers {
		out = append(out, p)
	}
	return out
}
