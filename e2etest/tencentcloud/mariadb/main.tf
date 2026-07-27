terraform {
  required_providers {
    tencentcloud = { source = "tencentcloudstack/tencentcloud", version = "~> 1.81" }
  }
}
provider "tencentcloud" { region = "ap-guangzhou" }

resource "tencentcloud_mariadb_instance" "prepaid" {
  zones      = ["ap-guangzhou-3", "ap-guangzhou-4"]
  memory     = 8
  storage    = 200
  node_count = 2
  period     = 1
}

resource "tencentcloud_mariadb_instance" "postpaid" {
  zones      = ["ap-guangzhou-3"]
  memory     = 8
  storage    = 200
  node_count = 2
}
