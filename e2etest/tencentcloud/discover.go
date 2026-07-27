// Spec discovery helper for products whose valid (zone, spec, version)
// combinations must be confirmed against the live API instead of guessed.
//
// Usage:
//
//	go run . --discover --products=postgresql,mongodb [--zone=ap-guangzhou-3]
//
// Requires: TENCENTCLOUD_SECRET_ID, TENCENTCLOUD_SECRET_KEY.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	mongodb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/mongodb/v20190725"
	postgres "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/postgres/v20170312"
	sqlserver "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sqlserver/v20180328"
)

// runDiscover prints live, sellable specs for the requested products so the
// TF/mapper configs can be pinned to real values.
func runDiscover(products []string, region, zone string) {
	secretID := os.Getenv("TENCENTCLOUD_SECRET_ID")
	secretKey := os.Getenv("TENCENTCLOUD_SECRET_KEY")
	if secretID == "" || secretKey == "" {
		fmt.Fprintln(os.Stderr, "discover: TENCENTCLOUD_SECRET_ID/KEY required")
		os.Exit(1)
	}
	cred := common.NewCredential(secretID, secretKey)

	want := map[string]bool{}
	for _, p := range products {
		want[strings.TrimSpace(p)] = true
	}
	if len(products) == 0 {
		want["postgresql"] = true
		want["mongodb"] = true
	}

	if want["postgresql"] {
		discoverPostgres(cred, region, zone)
		probePostgresPrice(cred, region, zone)
	}
	if want["mongodb"] {
		discoverMongo(cred, region, zone)
	}
	if want["sqlserver"] {
		probeSqlserverPrice(cred, region, zone)
	}
}

// probePostgresPrice calls InquiryPriceCreateDBInstances across every zone in
// the region to find where PostgreSQL is actually sellable. The "参数Zone State
// 检查失败" error means the zone is closed for sale even though DescribeClasses
// lists specs for it; only a live InquiryPrice call tells the truth.
func probePostgresPrice(cred *common.Credential, region, zone string) {
	cpf := profile.NewClientProfile()
	client, err := postgres.NewClient(cred, region, cpf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres client: %v\n", err)
		return
	}

	// First, list all zones in the region.
	fmt.Printf("\n=== PostgreSQL zones (region=%s) ===\n", region)
	zreq := postgres.NewDescribeZonesRequest()
	zresp, err := client.DescribeZones(zreq)
	var zones []string
	if err != nil {
		fmt.Fprintf(os.Stderr, "DescribeZones: %v\n", err)
		// fall back to a hardcoded guess list
		zones = []string{
			"ap-guangzhou-3", "ap-guangzhou-4", "ap-guangzhou-6",
			"ap-guangzhou-7", "ap-guangzhou-8",
		}
	} else {
		for _, z := range zresp.Response.ZoneSet {
			name := ""
			if z.Zone != nil {
				name = *z.Zone
			}
			state := ""
			if z.ZoneState != nil {
				state = *z.ZoneState
			}
			fmt.Printf("  Zone=%-18s State=%s Name=%s\n", name, state, derefStr(z.ZoneName))
			if name != "" {
				zones = append(zones, name)
			}
		}
	}

	fmt.Printf("\n=== PostgreSQL InquiryPrice probe (SpecCode=pg.it.small2) ===\n")
	// Only probe zones in the requested region (e.g. "ap-guangzhou-"); the
	// global DescribeZones returns every region and testing all of them is slow.
	regionPrefix := region + "-"
	for _, z := range zones {
		if !strings.HasPrefix(z, regionPrefix) {
			continue
		}
		req := postgres.NewInquiryPriceCreateDBInstancesRequest()
		req.Zone = common.StringPtr(z)
		req.SpecCode = common.StringPtr("pg.it.small2")
		req.Storage = common.Uint64Ptr(100)
		req.InstanceCount = common.Uint64Ptr(1)
		req.Period = common.Uint64Ptr(1)
		req.InstanceChargeType = common.StringPtr("PREPAID")
		req.DBEngine = common.StringPtr("postgresql")
		req.InstanceType = common.StringPtr("primary")
		resp, err := client.InquiryPriceCreateDBInstances(req)
		if err != nil {
			fmt.Printf("  [%-18s] ERROR: %v\n", z, err)
			continue
		}
		var orig, disc uint64
		if resp.Response.OriginalPrice != nil {
			orig = *resp.Response.OriginalPrice
		}
		if resp.Response.Price != nil {
			disc = *resp.Response.Price
		}
		fmt.Printf("  [%-18s] OK  Original=%d Price=%d Currency=%s\n",
			z, orig, disc, derefStr(resp.Response.Currency))
	}
}

func discoverPostgres(cred *common.Credential, region, zone string) {
	fmt.Printf("\n=== PostgreSQL sellable classes (zone=%s) ===\n", zone)
	cpf := profile.NewClientProfile()
	client, err := postgres.NewClient(cred, region, cpf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres client: %v\n", err)
		return
	}
	// Try a range of major versions; only the ones with classes in this zone print.
	for _, ver := range []string{"10", "11", "12", "13", "14", "15", "16"} {
		req := postgres.NewDescribeClassesRequest()
		req.Zone = common.StringPtr(zone)
		req.DBEngine = common.StringPtr("postgresql")
		req.DBMajorVersion = common.StringPtr(ver)
		resp, err := client.DescribeClasses(req)
		if err != nil {
			continue
		}
		classes := resp.Response.ClassInfoSet
		if len(classes) == 0 {
			continue
		}
		fmt.Printf("  -- DBMajorVersion=%s (%d classes)\n", ver, len(classes))
		type row struct {
			spec         string
			cpu, mem     uint64
			minSt, maxSt uint64
		}
		var rows []row
		for _, c := range classes {
			rows = append(rows, row{
				spec:  derefStr(c.SpecCode),
				cpu:   derefU64(c.CPU),
				mem:   derefU64(c.Memory),
				minSt: derefU64(c.MinStorage),
				maxSt: derefU64(c.MaxStorage),
			})
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].cpu != rows[j].cpu {
				return rows[i].cpu < rows[j].cpu
			}
			return rows[i].mem < rows[j].mem
		})
		for _, r := range rows {
			fmt.Printf("     SpecCode=%-28s CPU=%d Mem=%dMB Storage=%d-%dGB\n",
				r.spec, r.cpu, r.mem, r.minSt, r.maxSt)
		}
	}
}

func discoverMongo(cred *common.Credential, region, zone string) {
	fmt.Printf("\n=== MongoDB sellable specs (zone=%s) ===\n", zone)
	cpf := profile.NewClientProfile()
	client, err := mongodb.NewClient(cred, region, cpf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mongodb client: %v\n", err)
		return
	}
	req := mongodb.NewDescribeSpecInfoRequest()
	req.Zone = common.StringPtr(zone)
	resp, err := client.DescribeSpecInfo(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mongodb DescribeSpecInfo: %v\n", err)
		return
	}
	for _, si := range resp.Response.SpecInfoList {
		for _, it := range si.SpecItems {
			if derefU64(it.Status) != 1 {
				continue // only sellable
			}
			clusterType := "REPLSET"
			if derefU64(it.ClusterType) == 1 {
				clusterType = "SHARD"
			}
			fmt.Printf("  MachineType=%-7s Mem=%dGB Storage=%d-%dGB Version=%-13s Cluster=%s NodeNum=%d-%d\n",
				derefStr(it.MachineType),
				derefU64(it.Memory)/1024,
				derefU64(it.MinStorage)/1024, derefU64(it.MaxStorage)/1024,
				derefStr(it.MongoVersionCode),
				clusterType,
				derefU64(it.MinNodeNum), derefU64(it.MaxNodeNum))
		}
	}
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefU64(p *uint64) uint64 {
	if p == nil {
		return 0
	}
	return *p
}

// probeSqlserverPrice lists sqlserver zones via DescribeZones and probes
// InquiryPriceCreateDBInstances across each zone in several regions to find one
// that is actually sellable (not sold out).
func probeSqlserverPrice(cred *common.Credential, region, zone string) {
	cpf := profile.NewClientProfile()

	// Guangzhou is sold out for sqlserver; probe several major regions.
	regions := []string{"ap-guangzhou", "ap-beijing", "ap-shanghai", "ap-nanjing", "ap-chengdu", "ap-chongqing"}
	for _, reg := range regions {
		client, err := sqlserver.NewClient(cred, reg, cpf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sqlserver client (%s): %v\n", reg, err)
			continue
		}
		zreq := sqlserver.NewDescribeZonesRequest()
		zresp, err := client.DescribeZones(zreq)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [%s] DescribeZones: %v\n", reg, err)
			continue
		}
		regionPrefix := reg + "-"
		// Collect unique (zone, version) pairs; DescribeZones returns one entry
		// per (zone, spec, version) combo, so dedupe by zone+version.
		type zv struct{ zone, version string }
		var pairs []zv
		seen := map[zv]bool{}
		for _, z := range zresp.Response.ZoneSet {
			name := derefStr(z.Zone)
			ver := derefStr(z.Version)
			if name == "" || !strings.HasPrefix(name, regionPrefix) {
				continue
			}
			k := zv{name, ver}
			if seen[k] {
				continue
			}
			seen[k] = true
			pairs = append(pairs, k)
		}

		for _, p := range pairs {
			req := sqlserver.NewInquiryPriceCreateDBInstancesRequest()
			req.Zone = common.StringPtr(p.zone)
			req.Memory = common.Int64Ptr(4)
			req.Storage = common.Int64Ptr(100)
			req.InstanceChargeType = common.StringPtr("POSTPAID")
			req.GoodsNum = common.Int64Ptr(1)
			req.Period = common.Int64Ptr(1)
			req.InstanceType = common.StringPtr("HA")
			req.MachineType = common.StringPtr("CLOUD_PREMIUM")
			if p.version != "" {
				req.DBVersion = common.StringPtr(p.version)
			}
			resp, err := client.InquiryPriceCreateDBInstances(req)
			if err != nil {
				continue // skip sold-out / errors silently
			}
			var orig, price int64
			if resp.Response.OriginalPrice != nil {
				orig = *resp.Response.OriginalPrice
			}
			if resp.Response.Price != nil {
				price = *resp.Response.Price
			}
			fmt.Printf("  [region=%-14s zone=%-18s DBVersion=%-8s] OK  Original=%d Price=%d\n", reg, p.zone, p.version, orig, price)
		}
	}
	fmt.Println("(zones not listed above are sold out or errored)")
}

func derefI64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
