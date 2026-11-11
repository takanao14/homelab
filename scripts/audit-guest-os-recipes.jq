def require($ok): if $ok then . else error("invalid") end;
def host:
  require(type == "string") |
  require(test("^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$")) | split(".")[0] | ascii_downcase;
def release:
  if . == null then null else
    require(type == "string") |
    require(test("^[A-Za-z0-9_.-]+:[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")) |
    split("/")[-1] |
    if test("\\.(img|tar\\.zst)$") then
      [capture("^(?<os>ubuntu|debian|rocky|freebsd)-(?<version>[0-9]+(?:\\.[0-9]+)?)(?=[-_.])") | [.os, .version]][0] // null
    else null end
  end;
def matches($r; $o):
  $r[0] == $o[0] and
  (if $r[0] == "debian" or $r[0] == "rocky" then
     $o[1] == $r[1] or ($o[1] | startswith($r[1] + "."))
   elif $r[0] == "freebsd" then ($o[1] | split("-")[0]) == $r[1]
   else $o[1] == $r[1] end);

[.[] | require(type == "object") | to_entries[] |
  .key as $name | .value |
  require(type == "object") |
  require(keys == ["image", "kind", "node"]) |
  require(.kind == "vm" or .kind == "lxc") |
  (.node | host) as $node |
  {host: ($name | host), recipe: (.image | release)}] |
require(length == (map(.host) | unique | length)) as $recipes |
$observations | require(length == 1) | .[0] |
require(.status == "success" and .data.resultType == "vector") |
.data.result | require(type == "array") |
[.[] | .metric as $m |
  require((.value | type) == "array" and (.value | length) == 2) |
  require((.value[1] | tonumber) == 1) |
  $m.id as $id | $m.version_id as $v |
  require(($id | type) == "string" and ($v | type) == "string") |
  require(($id | test("^[a-z][a-z0-9_]*$")) and ($v | test("^[A-Za-z0-9.+_-]{1,64}$"))) |
  {host: ($m.nodename | host), os: [$id, $v]}] |
group_by(.host) | map({key: .[0].host, value: (map(.os) | unique)}) | from_entries as $observed |
[$recipes[] | . as $r | ($observed[.host] // []) as $o |
  . + {observed: (if ($o | length) == 1 then $o[0] else null end),
        status: (if .recipe == null then "unsupported-recipe"
                 elif ($o | length) == 0 then "unobserved"
                 elif ($o | length) > 1 then "ambiguous-observation"
                 elif matches($r.recipe; $o[0]) then "match" else "mismatch" end)}] |
sort_by(.host) as $rows |
($rows | map(select(.status == "match")) | length) as $matched |
($rows | map(select(.status == "mismatch")) | length) as $mismatched |
(($rows | length) - $matched - $mismatched) as $incomplete |
{rows: $rows, matched: $matched, mismatched: $mismatched, incomplete: $incomplete,
 exit: (if $mismatched > 0 then 1 elif $incomplete > 0 or ($rows | length) == 0 then 3 else 0 end)}
