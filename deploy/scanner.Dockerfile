FROM clamav/clamav:1.4.3
COPY deploy/clamd.conf /etc/clamav/clamd.conf
HEALTHCHECK --interval=10s --timeout=5s --start-period=180s --retries=10 \
    CMD test "$(printf 'PING\n' | nc -w 2 127.0.0.1 3310)" = "PONG"
