{{- define "homepage.config.settings" -}}
---
# https://gethomepage.dev/latest/configs/settings/
title: HomeLab Portal
description: Homelab Portal
logpath: stdout

{{- end }}

{{- define "homepage.config.services" -}}
---
# https://gethomepage.dev/latest/configs/services/

- DNS:
    - Authoritative:
        - PowerDNS auth1:
            icon: powerdns.png
            href: http://{{ .Values.pdns.primary.addr }}:8081/
            description: Primary Authoritative DNS ({{ .Values.pdns.primary.addr }}:1053)
            proxmoxNode: {{ .Values.pdns.primary.pveNode }}
            proxmoxVMID: {{ .Values.pdns.primary.pveVmid }}
            proxmoxType: {{ .Values.pdns.primary.pveType }}
            siteMonitor: http://{{ .Values.pdns.primary.addr }}:8081/

        - PowerDNS auth2:
            icon: powerdns.png
            href: http://{{ .Values.pdns.secondary.addr }}:8081/
            description: Secondary Authoritative DNS ({{ .Values.pdns.secondary.addr }}:1053)
            siteMonitor: http://{{ .Values.pdns.secondary.addr }}:8081/

    - DNS Balancer:
        - dnsdist1:
            icon: mdi-dns
            href: http://{{ .Values.dnsdist.dnsdist1.addr }}:8083/
            description: DNS Load Balancer ({{ .Values.dnsdist.dnsdist1.addr }}:53)
            proxmoxNode: {{ .Values.dnsdist.dnsdist1.pveNode }}
            proxmoxVMID: {{ .Values.dnsdist.dnsdist1.pveVmid }}
            proxmoxType: {{ .Values.dnsdist.dnsdist1.pveType }}
            siteMonitor: http://{{ .Values.dnsdist.dnsdist1.addr }}:8083/

        - dnsdist2:
            icon: mdi-dns
            href: http://{{ .Values.dnsdist.dnsdist2.addr }}:8083/
            description: DNS Load Balancer ({{ .Values.dnsdist.dnsdist2.addr }}:53)
            siteMonitor: http://{{ .Values.dnsdist.dnsdist2.addr }}:8083/

- VM/Storage:
    - HyperVisor:
        - Proxmox VE Prod:
            icon: proxmox.png
            href: {{ .Values.proxmox.prd.url }}
            description: Virtualization Platform
            widget:
              type: proxmox
              url: {{ .Values.proxmox.prd.url }}
              username: {{ .Values.proxmox.prd.username }}
              password: {{ .Values.proxmox.prd.password }}
              node: {{ .Values.proxmox.prd.node }}

        - Proxmox VE Dev:
            icon: proxmox.png
            href: {{ .Values.proxmox.dev.url }}
            description: Virtualization Platform
            widget:
              type: proxmox
              url: {{ .Values.proxmox.dev.url }}
              username: {{ .Values.proxmox.dev.username }}
              password: {{ .Values.proxmox.dev.password }}
              node: {{ .Values.proxmox.dev.node }}

        - Proxmox VE Prod2:
            icon: proxmox.png
            href: {{ .Values.proxmox.prd2.url }}
            description: Virtualization Platform
            widget:
              type: proxmox
              url: {{ .Values.proxmox.prd2.url }}
              username: {{ .Values.proxmox.prd2.username }}
              password: {{ .Values.proxmox.prd2.password }}
              node: {{ .Values.proxmox.prd2.node }}

    - Storage:
        - TrueNAS:
            icon: truenas.png
            href: {{ .Values.truenas.url }}
            description: Network Storage
            proxmoxNode: {{ .Values.truenas.node }}
            proxmoxVMID: {{ .Values.truenas.vmid }}
            widget:
              type: truenas
              url: {{ .Values.truenas.url }}
              key: {{ .Values.truenas.key }}
              nasType: scale

- Monitoring:
    - Grafana:
        icon: grafana.png
        href: {{ .Values.grafana.url }}
        description: Metrics Visualization
        widget:
          type: grafana
          version: 2
          url: {{ .Values.grafana.url }}
          username: {{ .Values.grafana.username }}
          password: {{ .Values.grafana.password }}

    - Prometheus:
        icon: prometheus.png
        href: {{ .Values.prometheus.url }}
        description: Monitoring System
        widget:
          type: prometheus
          url: {{ .Values.prometheus.url }}

- Develop:
    - forgejo:
        icon: forgejo.png
        href: {{ .Values.forgejo.url }}
        description: Git Repository

- Remote Management:
    - meshcentral:
        icon: si-intel-#0071C5
        href: {{ .Values.meshcentral.url }}
        description: MeshCentral AMT remote control

- Network Element:
    - bgw1:
        icon: mdi-router
        description: Border Gateway ({{ .Values.network.bgw1 }})

    - C1200:
        icon: mdi-switch
        href: http://{{ .Values.network.c1200 }}/
        description: L3-SW ({{ .Values.network.c1200 }})

    - WiFi-AP1:
        icon: mdi-access-point-network
        href: http://{{ .Values.network.wifiAp1 }}/
        description: wifi-ap1

    - WiFi-AP2:
        icon: mdi-access-point-network
        href: http://{{ .Values.network.wifiAp2 }}/
        description: wifi-ap2
{{- end }}

{{- define "homepage.config.proxmox" -}}

{{ .Values.proxmox.dev.node }}:
  url: {{ .Values.proxmox.dev.url }}
  token: {{ .Values.proxmox.dev.username }}
  secret: {{ .Values.proxmox.dev.password }}

{{ .Values.proxmox.prd.node }}:
  url: {{ .Values.proxmox.prd.url }}
  token: {{ .Values.proxmox.prd.username }}
  secret: {{ .Values.proxmox.prd.password }}

{{ .Values.proxmox.prd2.node }}:
  url: {{ .Values.proxmox.prd2.url }}
  token: {{ .Values.proxmox.prd2.username }}
  secret: {{ .Values.proxmox.prd2.password }}

{{- end }}

{{- define "homepage.config.widgets" -}}
---
# https://gethomepage.dev/latest/configs/widgets/

- kubernetes:
    cluster:
      show: true
      cpu: true
      memory: true
      showLabel: true
      label: "k0s Cluster"
    nodes:
      show: true
      cpu: true
      memory: true
      showLabel: true

{{- end }}
