terraform {
  required_providers {
    tencentcloud = { source = "tencentcloudstack/tencentcloud", version = "~> 1.81" }
  }
}
provider "tencentcloud" { region = "ap-guangzhou" }

resource "tencentcloud_eip" "test" {
  internet_max_bandwidth_out = 10
  internet_charge_type       = "TRAFFIC_POSTPAID_BY_HOUR"
}
