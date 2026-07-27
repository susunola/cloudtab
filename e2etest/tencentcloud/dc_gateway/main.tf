terraform {
  required_providers {
    tencentcloud = { source = "tencentcloudstack/tencentcloud", version = "~> 1.81" }
  }
}
provider "tencentcloud" { region = "ap-guangzhou" }

resource "tencentcloud_vpc" "test" {
  name       = "cloudtab-test-vpc"
  cidr_block = "10.0.0.0/16"
}

resource "tencentcloud_dc_gateway" "test" {
  name                = "cloudtab-test-dcg"
  network_type        = "VPC"
  network_instance_id = tencentcloud_vpc.test.id
}
