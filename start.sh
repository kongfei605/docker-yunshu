#!/bin/bash
docker rm -f yunshu-vpn 2>/dev/null || true
docker run -d \
  --name=yunshu-vpn \
  --hostname=yunshu-client \
  --device=/dev/net/tun \
  --cap-add=NET_ADMIN \
  --shm-size=512m \
  -e HTTP_PROXY= \
  -e HTTPS_PROXY= \
  -e http_proxy= \
  -e https_proxy= \
  -e NO_PROXY="localhost,127.0.0.1,.example.com,10.0.0.0/8,192.168.0.0/16,172.16.0.0/12,10.0.42.0/24" \
  -e no_proxy="localhost,127.0.0.1,.example.com,10.0.0.0/8,192.168.0.0/16,172.16.0.0/12,10.0.42.0/24" \
  -p 6903:6901 \
  -p 1091:1234 \
  -e VNC_PW=password \
  -e TZ=Asia/Shanghai \
  flashcatcloud/yunshu:v2.3.10.27
