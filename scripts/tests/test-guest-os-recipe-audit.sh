#!/usr/bin/env bash
set -euo pipefail
script_dir=$(cd "$(dirname "$0")/.." && pwd)
workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT
recipes="$workdir/recipes.json"
observed="$workdir/observed.json"
count=0

recipe() {
    jq -n --arg image "$1" '{demo: {kind: "vm", node: "node1", image: $image}}' > "$recipes"
}
observation() {
    jq -n --arg os "$1" --arg version "$2" \
      '{status:"success",data:{resultType:"vector",result:[{metric:{nodename:"DEMO.home.example",id:$os,version_id:$version},value:[0,"1"]}]}}' > "$observed"
}
check() {
    local expected=$1 status=$2 actual=0
    shift 2
    bash "$script_dir/audit-guest-os-recipes.sh" --recipes "$recipes" "$@" \
      --observations "$observed" > "$workdir/stdout" 2> "$workdir/stderr" || actual=$?
    [ "$actual" -eq "$expected" ] || { echo "Unexpected exit in case $count: $actual"; exit 1; }
    if [ -n "$status" ]; then
        grep -q "$status" "$workdir/stdout"
    fi
    if grep -q 'redaction-test-sentinel' "$workdir/stdout" "$workdir/stderr"; then
        echo 'Input was exposed.' >&2
        exit 1
    fi
    count=$((count + 1))
}

for entry in 'ubuntu 26.04 26.04' 'ubuntu 24.04 24.04' 'debian 13 13.6' 'rocky 10 10.1' 'freebsd 15.1 15.1-RELEASE'; do
    read -r os declared actual <<< "$entry"
    recipe "local:iso/$os-$declared-base.img"
    observation "$os" "$actual"
    check 0 match
done
recipe local:vztmpl/debian-13-standard_13.6-1_amd64.tar.zst
observation debian 13.6
check 0 match
observation ubuntu 26.04
check 1 mismatch
observation debian 130
check 1 mismatch
recipe local:iso/ubuntu-26.04-base.img
observation ubuntu 24.04
check 1 mismatch
observation ubuntu 26.04
jq '.data.result += .data.result' "$observed" > "$workdir/next.json"
mv "$workdir/next.json" "$observed"
check 0 match
jq '.data.result[1].metric.version_id = "24.04"' "$observed" > "$workdir/next.json"
mv "$workdir/next.json" "$observed"
check 3 ambiguous-observation
observation ubuntu 26.04
jq '.data.result[0].metric.nodename = "excluded-test"' "$observed" > "$workdir/next.json"
mv "$workdir/next.json" "$observed"
check 3 unobserved
recipe local:iso/custom.img
check 3 unsupported-recipe
recipe local:iso/ubuntu-26.04-base.img
observation ubuntu 26.04
jq '.data.result += [
  {metric:{nodename:"disposable",id:"debian",version_id:"12"},value:[0,"1"]},
  {metric:{nodename:"disposable",id:"ubuntu",version_id:"24.04"},value:[0,"1"]}
]' "$observed" > "$workdir/next.json"
mv "$workdir/next.json" "$observed"
check 0 match
check 2 '' "$recipes"
printf '{}\n' > "$workdir/empty.json"
check 0 match "$workdir/empty.json"
printf '{}\n' > "$recipes"
check 3 'Audited=0'
printf '{"demo":{"unexpected":"redaction-test-sentinel"}}\n' > "$recipes"
check 2 ''
printf 'redaction-test-sentinel\n' > "$recipes"
check 2 ''
recipe local:iso/ubuntu-26.04-base.img
printf 'redaction-test-sentinel\n' > "$observed"
check 2 ''
rm "$observed"
check 2 ''
echo "$count audit CLI cases passed."
