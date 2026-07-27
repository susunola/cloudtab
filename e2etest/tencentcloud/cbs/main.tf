terraform {
  required_providers {
    tencentcloud = { source = "tencentcloudstack/tencentcloud", version = "~> 1.81" }
  }
}
provider "tencentcloud" { region = "ap-guangzhou" }

resource "tencentcloud_cbs_storage" "postpaid" {
  storage_name      = "cloudtab-test-cbs"
  storage_type      = "CLOUD_PREMIUM"
  storage_size      = 100
  availability_zone = "ap-guangzhou-3"
  charge_type       = "POSTPAID_BY_HOUR"
}
