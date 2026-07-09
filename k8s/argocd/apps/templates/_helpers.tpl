{{/*
Environment name; rendering fails fast when it is not set.
*/}}
{{- define "argocd-apps.env" -}}
{{- required "Values.env must be set (prd / sandbox)" .Values.env -}}
{{- end }}
