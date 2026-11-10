output "guest_os_recipes" {
  description = "Creation template inputs for audits, not the running OS or proof of an OS update."
  value = {
    for name, container in var.containers : name => {
      kind  = "lxc"
      node  = container.node_name
      image = container.os_template
    }
  }
}
