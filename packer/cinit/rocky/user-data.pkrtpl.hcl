#cloud-config
users:
  - name: ${ssh_username}
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    lock_passwd: false
    ssh_authorized_keys:
      - ${ssh_pubkey}
    plain_text_passwd: ${user_password}
