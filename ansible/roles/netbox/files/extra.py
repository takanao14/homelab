# Mounted over the image's example extra.py; netbox-docker maps no environment
# variables to these. Caddy terminates TLS, so browser sessions are always HTTPS.
SESSION_COOKIE_SECURE = True
CSRF_COOKIE_SECURE = True
