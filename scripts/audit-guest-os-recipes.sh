#!/usr/bin/env bash
set -euo pipefail

invalid() {
    echo 'Invalid or unreadable audit input; export only guest_os_recipes and the documented OS query.' >&2
    exit 2
}
recipes=()
observations=
while [ "$#" -gt 0 ]; do
    case "$1" in
        --recipes)
            shift
            while [ "$#" -gt 0 ] && [[ "$1" != --* ]]; do
                recipes+=("$1")
                shift
            done
            ;;
        --observations)
            [ "$#" -ge 2 ] && [ -z "$observations" ] || invalid
            observations=$2
            shift 2
            ;;
        *) invalid ;;
    esac
done
[ "${#recipes[@]}" -gt 0 ] && [ -n "$observations" ] || invalid
command -v jq >/dev/null || { echo 'jq is required.' >&2; exit 2; }
script_dir=$(cd "$(dirname "$0")" && pwd)
# Suppress parser diagnostics: unexpected input may contain secrets.
if ! result=$(jq -cs --slurpfile observations "$observations" \
    -f "$script_dir/audit-guest-os-recipes.jq" -- "${recipes[@]}" 2>/dev/null); then
    invalid
fi
printf '%s\n' "$result" | jq -r '
  ["HOST", "STATUS", "RECIPE_OS", "OBSERVED_OS"],
  (.rows[] | [.host, .status, (.recipe // ["-"] | join(" ")), (.observed // ["-"] | join(" "))]) | @tsv'
printf '%s\n' "$result" | jq -r '"Audited=\(.rows|length) matched=\(.matched) mismatched=\(.mismatched) incomplete=\(.incomplete)"'
exit "$(printf '%s\n' "$result" | jq -r '.exit')"
