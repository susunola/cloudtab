terraform {
  required_providers {
    tencentcloud = { source = "tencentcloudstack/tencentcloud", version = "~> 1.81" }
  }
}
provider "tencentcloud" { region = "ap-guangzhou" }

resource "tencentcloud_vpn_gateway" "postpaid" {
  name        = "cloudtab-test-vpn-post"
  bandwidth   = 10
  charge_type = "POSTPAID_BY_HOUR"
}

resource "tencentcloud_vpn_gateway" "prepaid" {
  name           = "cloudtab-test-vpn-pre"
  bandwidth      = 20
  charge_type    = "PREPAID"
  prepaid_period = 1
}
