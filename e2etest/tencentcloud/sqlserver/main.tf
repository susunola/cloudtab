terraform {
  required_providers {
    tencentcloud = { source = "tencentcloudstack/tencentcloud", version = "~> 1.81" }
  }
}
provider "tencentcloud" { region = "ap-guangzhou" }

resource "tencentcloud_sqlserver_instance" "prepaid" {
  name              = "cloudtab-test-sqlserver-prepaid"
  availability_zone = "ap-guangzhou-6"
  engine_version    = "2022"
  memory            = 4
  storage           = 100
  charge_type       = "PREPAID"
  period            = 1
}

resource "tencentcloud_sqlserver_instance" "postpaid" {
  name              = "cloudtab-test-sqlserver-postpaid"
  availability_zone = "ap-guangzhou-6"
  engine_version    = "2022"
  memory            = 4
  storage           = 100
  charge_type       = "POSTPAID_BY_HOUR"
}
