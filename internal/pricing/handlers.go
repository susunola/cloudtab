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
	cloudhsm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cloudhsm/v20191112"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	cynosdb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cynosdb/v20190107"
	dcdb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/dcdb/v20180411"
	domain "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/domain/v20180808"
	ecm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ecm/v20190719"
	gaap "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/gaap/v20180529"
	lighthouse "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/lighthouse/v20200324"
	mariadb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/mariadb/v20170312"
	mongodb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/mongodb/v20190725"
	postgres "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/postgres/v20170312"
	redis "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/redis/v20180412"
	sqlserver "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sqlserver/v20180328"
	vpc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
	yunjing "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/yunjing/v20180228"
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
	"vpc": {
		product: "vpc",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return vpc.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			"InquiryPriceCreateVpnGateway": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := vpc.NewInquiryPriceCreateVpnGatewayRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*vpc.Client).InquiryPriceCreateVpnGateway(in)
				return sdkResult(out, err)
			},
		},
	},
	"mongodb": {
		product: "mongodb",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return mongodb.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			// Note: the SDK method is spelled "InquirePrice" (no 'y'), a known
			// naming quirk of the mongodb SDK. The action key mirrors it exactly.
			"InquirePriceCreateDBInstances": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := mongodb.NewInquirePriceCreateDBInstancesRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*mongodb.Client).InquirePriceCreateDBInstances(in)
				return sdkResult(out, err)
			},
		},
	},
	"mariadb": {
		product: "mariadb",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return mariadb.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			"DescribePrice": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := mariadb.NewDescribePriceRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*mariadb.Client).DescribePrice(in)
				return sdkResult(out, err)
			},
		},
	},
	"cynosdb": {
		product: "cynosdb",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return cynosdb.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			// SDK method is "InquirePriceCreate" (no 'y'); mirrored exactly.
			"InquirePriceCreate": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := cynosdb.NewInquirePriceCreateRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*cynosdb.Client).InquirePriceCreate(in)
				return sdkResult(out, err)
			},
		},
	},
	"lighthouse": {
		product: "lighthouse",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return lighthouse.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			// SDK method is "InquirePrice" (no 'y'), like mongodb/cynosdb.
			"InquirePriceCreateInstances": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := lighthouse.NewInquirePriceCreateInstancesRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*lighthouse.Client).InquirePriceCreateInstances(in)
				return sdkResult(out, err)
			},
		},
	},
	"ecm": {
		product: "ecm",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return ecm.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			"DescribePriceRunInstance": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := ecm.NewDescribePriceRunInstanceRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*ecm.Client).DescribePriceRunInstance(in)
				return sdkResult(out, err)
			},
		},
	},
	"sqlserver": {
		product: "sqlserver",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return sqlserver.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			"InquiryPriceCreateDBInstances": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := sqlserver.NewInquiryPriceCreateDBInstancesRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*sqlserver.Client).InquiryPriceCreateDBInstances(in)
				return sdkResult(out, err)
			},
		},
	},
	"dcdb": {
		product: "dcdb",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return dcdb.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			"DescribeDCDBPrice": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := dcdb.NewDescribeDCDBPriceRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*dcdb.Client).DescribeDCDBPrice(in)
				return sdkResult(out, err)
			},
		},
	},
	"gaap": {
		product: "gaap",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return gaap.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			"InquiryPriceCreateProxy": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := gaap.NewInquiryPriceCreateProxyRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*gaap.Client).InquiryPriceCreateProxy(in)
				return sdkResult(out, err)
			},
		},
	},
	"yunjing": {
		product: "yunjing",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return yunjing.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			"InquiryPriceOpenProVersionPrepaid": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := yunjing.NewInquiryPriceOpenProVersionPrepaidRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*yunjing.Client).InquiryPriceOpenProVersionPrepaid(in)
				return sdkResult(out, err)
			},
		},
	},
	"cloudhsm": {
		product: "cloudhsm",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return cloudhsm.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			"InquiryPriceBuyVsm": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := cloudhsm.NewInquiryPriceBuyVsmRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*cloudhsm.Client).InquiryPriceBuyVsm(in)
				return sdkResult(out, err)
			},
		},
	},
	"domain": {
		product: "domain",
		newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
			return domain.NewClient(cred, region, prof)
		},
		actions: map[string]actionInvoker{
			"DescribeDomainPriceList": func(client interface{}, params map[string]interface{}) ([]byte, error) {
				in := domain.NewDescribeDomainPriceListRequest()
				if err := bindParams(params, in); err != nil {
					return nil, err
				}
				out, err := client.(*domain.Client).DescribeDomainPriceList(in)
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
