terraform {
  required_providers {
    tencentcloud = { source = "tencentcloudstack/tencentcloud", version = "~> 1.81" }
  }
}
provider "tencentcloud" { region = "ap-guangzhou" }

resource "tencentcloud_clb_instance" "postpaid" {
  clb_name             = "cloudtab-test-clb"
  network_type         = "OPEN"
  internet_charge_type = "TRAFFIC_POSTPAID_BY_HOUR"
  sla_type             = "clb.c2.medium"
}
