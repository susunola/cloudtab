terraform {
  required_providers {
    tencentcloud = { source = "tencentcloudstack/tencentcloud", version = "~> 1.81" }
  }
}
provider "tencentcloud" { region = "ap-guangzhou" }

resource "tencentcloud_mysql_instance" "postpaid" {
  instance_name     = "cloudtab-test-postpaid"
  availability_zone = "ap-guangzhou-3"
  mem_size          = 4000
  volume_size       = 200
  charge_type       = "POSTPAID"
  cpu               = 2
}

resource "tencentcloud_mysql_instance" "prepaid" {
  instance_name     = "cloudtab-test-prepaid"
  availability_zone = "ap-guangzhou-3"
  mem_size          = 4000
  volume_size       = 200
  charge_type       = "PREPAID"
  prepaid_period    = 1
  cpu               = 2
}
