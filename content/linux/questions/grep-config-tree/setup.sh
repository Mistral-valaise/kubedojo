#!/usr/bin/env bash
# A small /etc-like tree for three audit questions. Every question has decoys:
# directives that are commented out, a disabled *.conf.bak with the worst
# settings of all, TLSv1.2 next to TLSv1, and addresses and numbers that only
# look like 10.20.x.x.
set -euo pipefail

base=/opt/course/grep-config-tree
rm -rf "$base"
e=$base/etc
mkdir -p "$e/nginx/sites" "$e/haproxy" "$e/app" "$e/mail"

cat > "$e/nginx/nginx.conf" <<'F'
user www-data;
worker_processes 4;
http {
    # ssl_protocols TLSv1 TLSv1.1 TLSv1.2;   (before the 2023 audit)
    ssl_protocols TLSv1.2 TLSv1.3;
    resolver 10.21.0.2;
    include sites/*.conf;
}
F
cat > "$e/nginx/sites/shop.conf" <<'F'
upstream shop_backend {
    server 10.20.3.7:8080;
    server 10.21.3.8:8080;
}
server {
    listen 443 ssl;
    server_name shop.example.com;
    ssl_protocols TLSv1 TLSv1.1 TLSv1.2;
    location / { proxy_pass http://shop_backend; }
}
F
cat > "$e/nginx/sites/blog.conf" <<'F'
server {
    listen 443 ssl;
    server_name blog.example.com;
    #ssl_protocols TLSv1.1 TLSv1.2;
    ssl_protocols TLSv1.2;
    location / { proxy_pass http://10.20.3.9:8080; }
    # old backend: proxy_pass http://10.20.3.4:8080;
}
F
cat > "$e/nginx/sites/api.conf" <<'F'
server {
    listen 443 ssl;
    server_name api.example.com;
	ssl_protocols TLSv1.1 TLSv1.2;
    location / { proxy_pass http://10.21.4.20:9000; }
}
F
cat > "$e/nginx/sites/default.conf" <<'F'
server {
    listen 80 default_server;
    # server_name _;
    return 444;
}
F
cat > "$e/nginx/sites/status.conf" <<'F'
server {
    listen 8081;
    location /status {
        stub_status;
        allow 110.20.1.5;
        allow 10.200.0.0/16;
        deny all;
    }
}
F
cat > "$e/nginx/sites/old-shop.conf.bak" <<'F'
server {
    listen 443 ssl;
    ssl_protocols TLSv1;
    location / { proxy_pass http://10.20.3.1:8080; }
}
F
cat > "$e/haproxy/haproxy.conf" <<'F'
global
    maxconn 10120
defaults
    timeout connect 10s
backend db
    server db1 10.20.8.4:5432 check
    server db2 10.20.8.5:5432 check backup
    # server db3 10.20.8.6:5432 check
F
cat > "$e/app/db.conf" <<'F'
[primary]
host = 10.20.8.4
port = 5432
# replica = 10.20.8.5
serial = 1012034
F
cat > "$e/app/cache.conf" <<'F'
[cache]
host = 10.21.0.3
ttl = 10
shards = 20
F
cat > "$e/mail/relay.conf" <<'F'
relayhost = 10.20.1.1
ssl_protocols TLSv1;
# server_name mail.example.com
F

(cd "$base" && find etc -type f -exec sha256sum {} + | LC_ALL=C sort -k2 | sha256sum | cut -d' ' -f1) > "$base/.manifest"
chown candidate:candidate "$base"
