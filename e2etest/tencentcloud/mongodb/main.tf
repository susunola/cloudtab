terraform {
  required_providers {
    tencentcloud = { source = "tencentcloudstack/tencentcloud", version = "~> 1.81" }
  }
}
provider "tencentcloud" { region = "ap-guangzhou" }

resource "tencentcloud_mongodb_instance" "prepaid" {
  available_zone  = "ap-guangzhou-3"
  instance_name   = "cloudtab-test-mongodb-prepaid"
  engine_version  = "MONGO_40_WT"
  machine_type    = "HIO"
  memory          = 4
  volume          = 100
  charge_type     = "PREPAID"
  prepaid_period  = 1
  node_num        = 3
}

resource "tencentcloud_mongodb_instance" "postpaid" {
  available_zone = "ap-guangzhou-3"
  instance_name  = "cloudtab-test-mongodb-postpaid"
  engine_version = "MONGO_40_WT"
  machine_type   = "HIO"
  memory         = 4
  volume         = 100
  charge_type    = "POSTPAID_BY_HOUR"
}
