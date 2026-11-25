#!/usr/bin/env bash

# Resolve OpenBao auth from BAO_TOKEN, token stdin, BAO_PASSWORD, or a prompt.

openbao_authenticate() {
  if [[ -n "${BAO_TOKEN:-}" ]]; then
    export BAO_TOKEN
    return
  fi

  if [[ "${BAO_TOKEN_STDIN:-}" == 1 ]]; then
    read -r BAO_TOKEN
    export BAO_TOKEN
    return
  fi

  if [[ -n "${BAO_PASSWORD:-}" ]]; then
    _bao_pass="$BAO_PASSWORD"
  elif [[ -t 0 ]]; then
    read -rsp "OpenBao password for ${BAO_USERNAME}: " _bao_pass; echo
  elif ! IFS= read -r _bao_pass || [[ -z "${_bao_pass}" ]]; then
    echo "Error: OpenBao password is required on stdin when BAO_TOKEN is unset" >&2
    return 1
  fi

  if ! command -v jq >/dev/null 2>&1; then
    echo "Error: jq CLI not found" >&2
    return 1
  fi

  BAO_TOKEN="$(
    printf '%s' "${_bao_pass}" |
      jq -Rsc '{password: .}' |
      bao write -format=json "auth/userpass/login/${BAO_USERNAME}" - |
      jq -er '.auth.client_token'
  )"
  export BAO_TOKEN
}
