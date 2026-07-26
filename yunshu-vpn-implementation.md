# YunShu VPN Docker Image Implementation Plan / 云枢VPN Docker镜像制作方案

## Goal Description / 目标说明

This plan outlines the steps required to transform the existing `docker-corplink` build process to support the **YunShu VPN** (云枢VPN) client using the provided `YunShu_2.3.10.27_MCD.deb` package on the remote machine (10.129.0.69).

The previous `corplink` image is based on `kasmweb/core-ubuntu-focal` (which provides a headless KasmVNC workspace) and sets up proxy services (privoxy, socks5) alongside the VPN daemon and GUI. The goal is to replicate this architecture for YunShu.

> [!NOTE]
> We have successfully reverse-engineered the YunShu DEB package to find the GUI executable (`/opt/apps/com.eagleyun.yunshu/files/yunshu-cross --disable-http2`), the daemon executable (`/opt/apps/com.eagleyun.yunshu/files/bin/yunshu-daemon`), and the desktop shortcut location.

## Open Questions / 待确认问题

> [!WARNING]
> 1. **DNS Fix Script (fixdns.sh)**: In the Corplink image, there was a script that continuously read `/opt/Corplink/vpn.conf` to extract the DNS server IP and overwrite `/etc/resolv.conf`. Does YunShu also require this automatic DNS overwrite? If yes, what is the path to the YunShu configuration file that stores the DNS IP?
> 2. **Execution Environment**: We will execute these changes directly on the remote machine (`10.129.0.69`) in the directory `/home/kongfei/go/src/github.com/flashcatcloud/docker-yunshu`. Please confirm this is acceptable.

## Proposed Changes / 计划变更

### 1. File & Directory Renames / 文件及目录重命名
Rename references of `corplink` to `yunshu` to maintain a clean project structure.

#### [NEW] `src/ubuntu/install/yunshu/` (Renamed from `src/ubuntu/install/corplink/`)
#### [NEW] `src/ubuntu/install/yunshu/install_yunshu.sh` (Renamed from `install_corplink.sh`)
#### [DELETE] `src/ubuntu/install/corplink/`

---

### 2. Dockerfile & Build Scripts / Dockerfile及构建脚本更新

#### [MODIFY] `Dockerfile`
- Add `COPY YunShu_2.3.10.27_MCD.deb $INST_SCRIPTS/yunshu/yunshu.deb` to include the package locally instead of downloading via `wget`.
- Update `COPY` and `RUN` commands to point to the new `yunshu` paths.

#### [MODIFY] `Makefile`
- Update the build command to `docker build -t flashcatcloud/yunshu:v2.3.10.27 .`

#### [MODIFY] `start.sh` & `test.sh`
- Update the container name and image reference to `flashcatcloud/yunshu:v2.3.10.27`.

#### [MODIFY] `README.md`
- Update documentation to reflect YunShu installation and usage.

---

### 3. Startup & Installation Scripts / 启动及安装脚本配置

#### [MODIFY] `src/ubuntu/install/yunshu/install_yunshu.sh`
- **Supervisor Config**: Update the `corplink` supervisor service to `yunshu`, setting the command to `/opt/apps/com.eagleyun.yunshu/files/bin/yunshu-daemon`.
- **DNS Fix**: Temporarily disable or adjust the `fixdns.sh` supervisor configuration until the YunShu config path is known.
- **Package Installation**: Change the installation command to `apt install ./yunshu.deb -y`.
- **Desktop Shortcut**: Copy from `/opt/apps/com.eagleyun.yunshu/entries/applications/com.eagleyun.yunshu.desktop`.

#### [MODIFY] `src/ubuntu/install/yunshu/custom_startup.sh`
- Update `START_COMMAND="/opt/apps/com.eagleyun.yunshu/files/yunshu-cross --disable-http2"`.
- Update `PGREP="yunshu-cross"`.
- Update `MAXIMIZE_NAME="YunShu"`.

#### [MODIFY] `src/ubuntu/install/yunshu/manage_routes.sh`
- Ensure it is executed via `custom_startup.sh`. No major code changes are expected here unless YunShu uses a different TUN interface name (Corplink used `utun` or similar standard tun interfaces).

## Verification Plan / 验证计划

### Automated Tests
- Run `make build` on the remote server to ensure the Docker image builds successfully without errors.

### Manual Verification
- Execute `start.sh` to spin up the container.
- Connect to `https://10.129.0.69:6901` to verify the KasmVNC workspace boots and the YunShu GUI is automatically launched.
- Verify that `supervisorctl status` inside the container shows `yunshu` and proxy services running.
