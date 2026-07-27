terraform {
  required_providers {
    tencentcloud = { source = "tencentcloudstack/tencentcloud", version = "~> 1.81" }
  }
}
provider "tencentcloud" { region = "ap-guangzhou" }

resource "tencentcloud_dcdb_db_instance" "prepaid" {
  zones            = ["ap-guangzhou-3", "ap-guangzhou-4"]
  shard_memory     = 8
  shard_storage    = 200
  shard_count      = 2
  shard_node_count = 2
  period           = 1
}

resource "tencentcloud_dcdb_hourdb_instance" "postpaid" {
  zones            = ["ap-guangzhou-3"]
  shard_memory     = 8
  shard_storage    = 200
  shard_count      = 2
  shard_node_count = 2
}
