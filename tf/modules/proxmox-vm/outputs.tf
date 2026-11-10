output "guest_os_recipes" {
  description = "Creation image inputs for audits, not the running OS or proof of an OS update."
  value = {
    for name, vm in var.vms : name => {
      kind  = "vm"
      node  = vm.node_name
      image = try(vm.disks[sort(keys(vm.disks))[0]].file_id, null)
    }
  }
}
