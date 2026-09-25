# Butako voice chat

The prd Argo CD Application deploys the Go/TypeScript app and VOICEVOX CPU
engine in the `butako-voice` namespace. The HTTPRoute exposes
`https://butaco.prd.butaco.net`; ExternalDNS creates its PowerDNS record.
The app calls Lemonade and VOICEVOX through ClusterIP Services.

`values.yaml` pins the app release tag and the official VOICEVOX 0.25.2 amd64
digest. VOICEVOX's entrypoint prints requests containing the synthesis text,
so the container redirects stdout and stderr to `/dev/null`. Inspect Pod and
node log files after deployment to confirm no conversation text is retained.

The route, API, and browser use a 120-second conversation deadline. The API
rejects input over 2 MiB and responses over 2 MiB. The UI requires HTTPS for
Safari microphone access. A SecurityPolicy denies requests outside
`192.168.10.0/24`; confirm the Gateway sees the expected client IP before
accepting the unauthenticated route.
