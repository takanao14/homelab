FROM ghcr.io/helmfile/helmfile:v1.4.4

COPY --from=quay.io/argoproj/argocd:v3.3.6 \
    /usr/local/bin/argocd-cmp-server \
    /usr/local/bin/argocd-cmp-server

USER 999
ENTRYPOINT ["/usr/local/bin/argocd-cmp-server"]
