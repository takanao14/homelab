{{/*
Environment name; rendering fails fast when it is not set.
*/}}
{{- define "argocd-apps.env" -}}
{{- required "Values.env must be set (prd / sandbox)" .Values.env -}}
{{- end }}

{{/* Preserve only Reloader's restart marker during comparison and sync. */}}
{{- define "argocd-apps.reloaderIgnoreDifferences" -}}
ignoreDifferences:
  - group: apps
    kind: Deployment
    namespace: {{ .namespace }}
    {{- with .name }}
    name: {{ . }}
    {{- end }}
    jsonPointers:
      - /spec/template/metadata/annotations/reloader.stakater.com~1last-reloaded-from
{{- end }}
