FROM caddy:2.10.2-alpine
RUN setcap -r /usr/bin/caddy && chown -R 1000:1000 /data /config
USER 1000:1000
