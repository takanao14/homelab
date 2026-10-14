# reloader-check

Permanent sandbox-only consumers for ADR-0071 acceptance checks. The opt-in
Deployments exercise client-side and server-side Argo CD apply; the control
Deployment has no Reloader opt-in. All mount the same optional Secret and stay
Healthy without it. No Secret or ExternalSecret is managed by this chart.

Supply `Secret/reloader-check` temporarily through ESO using dummy values in
OpenBao `secret/k8s-sandbox/reloader-check`. After ESO is ready, record Deployment
revisions before changing the value. Both opt-in consumers must roll once for
changed data and never for unchanged data; the control must remain unchanged.
An unrelated Argo CD sync must preserve the restart marker while other Pod
template drift remains visible.

Delete the temporary ExternalSecret, generated Secret, and OpenBao KV metadata
after validation. Keep this Application for cross-namespace bootstrap checks.
