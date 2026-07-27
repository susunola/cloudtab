terraform {
  required_providers {
    tencentcloud = { source = "tencentcloudstack/tencentcloud", version = "~> 1.81" }
  }
}
provider "tencentcloud" { region = "ap-guangzhou" }

data "tencentcloud_images" "ubuntu" {
  image_type = ["PUBLIC_IMAGE"]
  os_name    = "ubuntu"
}

resource "tencentcloud_instance" "postpaid" {
  instance_type        = "SA2.MEDIUM4"
  image_id             = data.tencentcloud_images.ubuntu.images.0.image_id
  availability_zone    = "ap-guangzhou-3"
  instance_charge_type = "POSTPAID_BY_HOUR"
  system_disk_type     = "CLOUD_PREMIUM"
  system_disk_size     = 50
}

resource "tencentcloud_instance" "prepaid" {
  instance_type                       = "SA2.MEDIUM4"
  image_id                            = data.tencentcloud_images.ubuntu.images.0.image_id
  availability_zone                   = "ap-guangzhou-3"
  instance_charge_type                = "PREPAID"
  instance_charge_type_prepaid_period = 1
  system_disk_type                    = "CLOUD_PREMIUM"
  system_disk_size                    = 50
}
