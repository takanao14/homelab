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
  else
    read -r _bao_pass
  fi

  BAO_TOKEN=$(bao login -token-only -method=userpass username="${BAO_USERNAME}" password="${_bao_pass}")
  export BAO_TOKEN
}
