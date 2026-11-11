locals {
  guest_os_recipes = {
    for name, container in var.containers : name => {
      kind  = "lxc"
      node  = container.node_name
      image = container.os_template
    } if container.os_recipe_audit
  }
}

output "guest_os_recipes" {
  description = "Opted-in creation template inputs for audits, not the running OS or proof of an OS update."
  value       = length(local.guest_os_recipes) > 0 ? local.guest_os_recipes : null
}
