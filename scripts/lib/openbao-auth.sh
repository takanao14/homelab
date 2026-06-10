#!/usr/bin/env bash

# Shared OpenBao auth helper sourced by the secrets/ scripts. Resolves a token in
# order: BAO_TOKEN env -> stdin (BAO_TOKEN_STDIN=1) -> BAO_PASSWORD env ->
# interactive prompt or stdin password via userpass login.

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
