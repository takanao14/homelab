locals {
  guest_os_recipes = {
    for name, vm in var.vms : name => {
      kind  = "vm"
      node  = vm.node_name
      image = try(vm.disks[sort(keys(vm.disks))[0]].file_id, null)
    } if vm.os_recipe_audit
  }
}

output "guest_os_recipes" {
  description = "Opted-in creation image inputs for audits, not the running OS or proof of an OS update."
  value       = length(local.guest_os_recipes) > 0 ? local.guest_os_recipes : null
}
