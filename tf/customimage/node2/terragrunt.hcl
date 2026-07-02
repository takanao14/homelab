include "root" {
  path = find_in_parent_folders("root.hcl")
}

include "base" {
  path = find_in_parent_folders("base.hcl")
}
