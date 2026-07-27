package main

// Product test case definitions for all 19 Tencent Cloud resource types.
// Each product maps to a directory under e2etest/tencentcloud/ and a Validator.

import (
	"strings"

	"github.com/susunola/cloudtab/internal/pricing"
)

// TestCase defines one product's E2E test configuration.
type TestCase struct {
	Name         string // directory name, e.g. "cvm"
	ResourceType string // Terraform type, e.g. "tencentcloud_instance"
	Validator    Validator
}

// allTestCases returns all 19 Tencent Cloud product test cases.
func allTestCases() []TestCase {
	return []TestCase{
		{Name: "cvm", ResourceType: "tencentcloud_instance", Validator: cvmClbValidator{}},
		{Name: "cbs", ResourceType: "tencentcloud_cbs_storage", Validator: cbsValidator{}},
		{Name: "eip", ResourceType: "tencentcloud_eip", Validator: eipValidator{}},
		{Name: "clb", ResourceType: "tencentcloud_clb_instance", Validator: cvmClbValidator{}},
		{Name: "mysql", ResourceType: "tencentcloud_mysql_instance", Validator: cdbFenValidator{}},
		{Name: "postgresql", ResourceType: "tencentcloud_postgresql_instance", Validator: cdbFenValidator{}},
		{Name: "redis", ResourceType: "tencentcloud_redis_instance", Validator: redisValidator{}},
		{Name: "vpn_gateway", ResourceType: "tencentcloud_vpn_gateway", Validator: vpnValidator{}},
		{Name: "dc_gateway", ResourceType: "tencentcloud_dc_gateway", Validator: dcgValidator{}},
		{Name: "mongodb", ResourceType: "tencentcloud_mongodb_instance", Validator: mongodbValidator{}},
		{Name: "mariadb", ResourceType: "tencentcloud_mariadb_instance", Validator: cdbFenValidator{}},
		{Name: "cynosdb", ResourceType: "tencentcloud_cynosdb_cluster", Validator: cynosdbValidator{}},
		{Name: "lighthouse", ResourceType: "tencentcloud_lighthouse_instance", Validator: lighthouseValidator{}},
		{Name: "ecm", ResourceType: "tencentcloud_ecm_instance", Validator: ecmValidator{}},
		{Name: "sqlserver", ResourceType: "tencentcloud_sqlserver_instance", Validator: cdbFenValidator{}},
		{Name: "dcdb", ResourceType: "tencentcloud_dcdb_instance", Validator: cdbFenValidator{}},
		{Name: "gaap", ResourceType: "tencentcloud_gaap_proxy", Validator: gaapValidator{}},
		{Name: "cwp", ResourceType: "tencentcloud_cwp_license_order", Validator: cwpValidator{}},
		{Name: "cloudhsm", ResourceType: "tencentcloud_cloudhsm_instance", Validator: cloudhsmValidator{}},
		{Name: "domain", ResourceType: "tencentcloud_domain_registration", Validator: domainValidator{}},
	}
}

// isPrepaid returns true if the request params indicate PREPAID billing.
func isPrepaid(req pricing.PriceRequest) bool {
	// Redis uses BillingMode (int64): 0=postpaid, 1=prepaid.
	if bm, ok := req.Params["BillingMode"]; ok {
		switch v := bm.(type) {
		case int64:
			if v == 1 {
				return true
			}
		case int:
			if v == 1 {
				return true
			}
		}
	}
	for _, key := range []string{"InstanceChargeType", "InstanceChargePrepaid", "ChargePrepaid", "ChargeType", "InstanceChargeTypePrepaidPeriod", "InternetChargeType"} {
		if v, ok := req.Params[key]; ok {
			s := ""
			switch vv := v.(type) {
			case string:
				s = vv
			case map[string]interface{}:
				s = "" // prepaid config block presence implies prepaid
				return true
			}
			if strings.Contains(strings.ToUpper(s), "PREPAID") {
				return true
			}
		}

	}
	// Check common field names used by various mappers
	if ct, ok := req.Params["Paymode"].(string); ok && strings.Contains(strings.ToUpper(ct), "PREPAID") {
		return true
	}
	if ct, ok := req.Params["InstanceChargeType"].(string); ok {
		if ct == "PREPAID" || strings.Contains(ct, "PREPAID") {
			return true
		}
	}
	return false
}
