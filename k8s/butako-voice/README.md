# Butako voice chat

The prd Argo CD Application deploys the Go/TypeScript app and VOICEVOX CPU
engine in the `butako-voice` namespace. The HTTPRoute exposes
`https://butaco.prd.butaco.net`; ExternalDNS creates its PowerDNS record.
The app calls Lemonade and VOICEVOX through ClusterIP Services.

`chart/values.yaml` `character` sets the persona, ASR vocabulary hint, VOICEVOX
speaker, UI name and credit, speech replacements, and hallucination list. The
chart renders it to the `butako-character` ConfigMap as the app's
`CHARACTER_FILE`; a checksum annotation restarts the app when it changes. The
app rejects unknown keys at startup, so a typo shows as a failed rollout.

`character.football` makes the app fetch match facts for the listed ESPN team
IDs from ESPN's unofficial API every 30 minutes and append them to the system
prompt (ADR-0056). Requests carry only fixed league slugs and IDs, never user
speech. Removing the key disables the feature. Deploy an app version that
knows the key before adding it, or the app refuses to start.

`values.yaml` pins the app release tag and the official VOICEVOX 0.25.2 amd64
digest. VOICEVOX's entrypoint prints requests containing the synthesis text,
so the container redirects stdout and stderr to `/dev/null`. Inspect Pod and
node log files after deployment to confirm no conversation text is retained.

The route, API, and browser use a 120-second conversation deadline. The API
rejects input over 2 MiB and responses over 2 MiB. The UI requires HTTPS for
Safari microphone access. A SecurityPolicy denies requests outside
`192.168.10.0/24`; confirm the Gateway sees the expected client IP before
accepting the unauthenticated route.
