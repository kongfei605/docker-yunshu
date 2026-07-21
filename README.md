# docker-yunshu
docker-yunshu builds a docker image to run the YunShu VPN client in a container, which is based on [kasmtech/workspaces-images](https://github.com/kasmtech/workspaces-images).

## Usage

1. Run the container as a daemon.

```bash
docker run -d \
  --name=yunshu-vpn \
  --hostname=yunshu-client \
  --device=/dev/net/tun \
  --cap-add=NET_ADMIN \
  --shm-size=512m \
  -p 6901:6901 \
  -p 1094:1234 \
  -e VNC_PW=password \
  -e TZ=Asia/Shanghai \
  flashcatcloud/yunshu:v2.3.10.27
```

2. Configure YunShu via a browser: https://127.0.0.1:6901.

* User : kasm_user
* Password: password

3. Access YunShu VPN network via HTTP proxy: 127.0.0.1:8118 or SOCKS5 proxy: 127.0.0.1:1234.

> **Note:** This container provides the core VPN daemon and GUI capabilities. Advanced terminal security capabilities included in the native YunShu package (like DLP, screenshot protection, and udev control) are explicitly disabled or omitted to allow execution inside a standard unprivileged Docker environment.

## Acknowledgments

* Thanks [kasmtech](https://github.com/kasmtech) for providing some great open source tools.
