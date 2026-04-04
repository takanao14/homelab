locals {
  dns_internal  = ["192.168.10.242", "192.168.10.241"]
  dns_external  = ["192.168.10.1", "8.8.8.8"]
  dns_domain    = "home.butaco.net"

  dev = {
    net10 = {
      bridge = "vmbr0"
      ipv4gw = "192.168.10.1"
    }
    net20 = {
      bridge = "vnets001"
      ipv4gw = "192.168.20.1"
    }
  }

  prd = {
    net10 = {
      bridge = "vmbr0"
      ipv4gw = "192.168.10.1"
    }
    net30 = {
      bridge = "vnets30"
      ipv4gw = "192.168.30.1"
    }
  }

  prd2 = {
    net10 = {
      bridge = "vmbr0"
      ipv4gw = "192.168.10.1"
    }
    net40 = {
      bridge = "vnets40"
      ipv4gw = "192.168.40.1"
    }
  }
}
