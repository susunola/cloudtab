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
  availability_zone = "ap-guangzhou-3"
}

resource "tencentcloud_cynosdb_cluster" "prepaid" {
  cluster_name    = "cloudtab-test-cynosdb"
  available_zone  = "ap-guangzhou-3"
  db_type         = "MYSQL"
  db_version      = "5.7"
  password        = "Cloudtab@Test123"
  vpc_id          = tencentcloud_vpc.test.id
  subnet_id       = tencentcloud_subnet.test.id
  instance_cpu_core    = 2
  instance_memory_size = 4
  storage_limit   = 100
  charge_type     = "PREPAID"
  prepaid_period  = 1
  instance_count  = 1
}

resource "tencentcloud_cynosdb_cluster" "postpaid" {
  cluster_name    = "cloudtab-test-cynosdb-post"
  available_zone  = "ap-guangzhou-3"
  db_type         = "MYSQL"
  db_version      = "5.7"
  password        = "Cloudtab@Test123"
  vpc_id          = tencentcloud_vpc.test.id
  subnet_id       = tencentcloud_subnet.test.id
  instance_cpu_core    = 2
  instance_memory_size = 4
  charge_type     = "POSTPAID_BY_HOUR"
}

resource "tencentcloud_cynosdb_cluster" "serverless" {
  cluster_name    = "cloudtab-test-cynosdb-serverless"
  available_zone  = "ap-guangzhou-3"
  db_type         = "MYSQL"
  db_mode         = "SERVERLESS"
  db_version      = "5.7"
  password        = "Cloudtab@Test123"
  vpc_id          = tencentcloud_vpc.test.id
  subnet_id       = tencentcloud_subnet.test.id
  min_cpu         = 1
  max_cpu         = 2
  charge_type     = "POSTPAID_BY_HOUR"
}
