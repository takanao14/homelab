{{/*
Environment name; rendering fails fast when it is not set.
*/}}
{{- define "argocd-apps.env" -}}
{{- required "Values.env must be set (prd / dev / sandbox)" .Values.env -}}
{{- end }}
