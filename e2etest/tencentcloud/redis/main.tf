terraform {
  required_providers {
    tencentcloud = { source = "tencentcloudstack/tencentcloud", version = "~> 1.81" }
  }
}
provider "tencentcloud" { region = "ap-guangzhou" }

resource "tencentcloud_redis_instance" "postpaid" {
  availability_zone  = "ap-guangzhou-3"
  type_id            = 6
  mem_size           = 1024
  charge_type        = "POSTPAID"
  redis_shard_num    = 1
  redis_replicas_num = 1
}

resource "tencentcloud_redis_instance" "prepaid" {
  availability_zone  = "ap-guangzhou-3"
  type_id            = 6
  mem_size           = 1024
  charge_type        = "PREPAID"
  prepaid_period     = 1
  redis_shard_num    = 1
  redis_replicas_num = 1
}
