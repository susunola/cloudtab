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

resource "tencentcloud_subnet" "test" {
  vpc_id            = tencentcloud_vpc.test.id
  name              = "cloudtab-test-subnet"
  cidr_block        = "10.0.1.0/24"
  availability_zone = "ap-guangzhou-4"
}

resource "tencentcloud_postgresql_instance" "postpaid" {
  name              = "cloudtab-test-pg"
  availability_zone = "ap-guangzhou-4"
  memory            = 2
  storage           = 100
  vpc_id            = tencentcloud_vpc.test.id
  subnet_id         = tencentcloud_subnet.test.id
  root_password     = "Cloudtab@Test123"
  charge_type       = "POSTPAID_BY_HOUR"
}
