locals {
  templates = {
    trixie   = "local:vztmpl/debian-13-standard_13.6-1_amd64.tar.zst"
    noble    = "local:vztmpl/ubuntu-24.04-standard_24.04-2_amd64.tar.zst"
    resolute = "local:vztmpl/ubuntu-26.04-standard_26.04-1_amd64.tar.zst"
  }

  os_types = {
    noble    = "ubuntu"
    resolute = "ubuntu"
    trixie   = "debian"
  }

  releases = {
    ns1       = "resolute"
    ns2       = "resolute"
    ns3       = "resolute"
    dist1     = "resolute"
    dist2     = "resolute"
    resolver1 = "resolute"
    resolver2 = "resolute"
  }
}
