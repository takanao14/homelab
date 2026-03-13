# Homelab Ansible Automation

This directory contains Ansible playbooks and roles used to provision, configure, and manage the infrastructure of the homelab environment.

## Directory Structure

- `ansible.cfg`: Default Ansible configuration.
- `.envrc.sample`: A template for environment variables.
- `inventories/`: Inventory files defining hosts and groups.
  - `homelab/hosts.yml.sample`: A sample inventory file.
- `playbooks/`: High-level playbooks that map roles to host groups.
- `roles/`: Reusable Ansible roles for specific services.

## Getting Started

### 1. Create Inventory File
The main inventory file `inventories/homelab/hosts.yml` is ignored by Git to protect sensitive information. To get started, copy the sample file and edit it to match your environment.

```bash
cd ansible
cp inventories/homelab/hosts.yml.sample inventories/homelab/hosts.yml
vi inventories/homelab/hosts.yml # Edit IPs and usernames
```

### 2. Set Up Environment Variables
This project uses `direnv` to manage environment variables for sensitive data (passwords, API keys). Copy the sample file, fill in your values, and then enable it.

```bash
cp .envrc.sample .envrc
vi .envrc # Edit your secret values
direnv allow
```

### 3. Run Playbooks
Once your inventory and environment variables are set, you can run the playbooks.

```bash
# Example: Run the DNS playbook
ansible-playbook playbooks/dns.yml

# Example: Run the Node Exporter playbook
ansible-playbook playbooks/node_exporter.yml
```

## Playbooks

A brief overview of the available playbooks. For details on required variables, see `.envrc.sample`.

### DNS (`playbooks/dns.yml`)
Deploys and configures the homelab DNS infrastructure using PowerDNS Authoritative Server and dnsdist.
- **Roles used:** `dnsdist`, `pdns_auth`
- **Target hosts:** `dns_primary`, `dns_secondary`

### Node Exporter (`playbooks/node_exporter.yml`)
Installs and configures `prometheus-node-exporter` for metric collection.
- **Roles used:** `node_exporter`
- **Target hosts:** `node_exporter` group

### Proxmox Configuration (`playbooks/proxmox.yml`)
Configures baseline settings on Proxmox VE hosts, such as creating a maintenance user.
- **Target hosts:** `proxmox`

## Tips
- Use `ansible-playbook <playbook> --check` for a dry run.
- Ensure your local SSH keys are properly loaded (`ssh-add`) to connect to the target machines.
