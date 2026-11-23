# arrstart bootstrap/wizard image (WP0.12 placeholder).
#
# Minimal image published to ${DefaultRegistry} (ADR-0010). The real wizard
# binary is wired in here once the Go backend (cmd/arrstart) exists. No
# "latest" tag is ever published -- image tags are pinned to repo versions.
FROM alpine:3.20

LABEL org.opencontainers.image.source="https://github.com/bruceowenga/arrstart"
LABEL org.opencontainers.image.title="arrstart"
LABEL org.opencontainers.image.description="arrstart bootstrap/wizard container image (placeholder)"
LABEL org.opencontainers.image.vendor="bruceowenga"

CMD ["/bin/sh"]
