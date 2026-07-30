ARG GO_BUILDER_IMAGE="golang:1.23-bookworm"
ARG BASE_TAG="1.14.0"
ARG BASE_IMAGE="core-ubuntu-focal"
FROM ${GO_BUILDER_IMAGE} AS yunshu-go-builder
WORKDIR /src/yunshu
COPY ./src/ubuntu/install/yunshu/ ./
RUN go mod download && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/auto_mfa_daemon ./cmd/auto_mfa_daemon && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/get_otp_secret ./cmd/get_otp_secret

FROM hub.witd.in/kasmweb/$BASE_IMAGE:$BASE_TAG
USER root

ENV HOME /home/kasm-default-profile
ENV STARTUPDIR /dockerstartup
ENV INST_SCRIPTS $STARTUPDIR/install
WORKDIR $HOME

######### Customize Container Here ###########

# Install Google Chrome
COPY ./src/ubuntu/install/chrome $INST_SCRIPTS/chrome/
RUN bash $INST_SCRIPTS/chrome/install_chrome.sh  && rm -rf $INST_SCRIPTS/chrome/

# Install dependencies
RUN apt-get update && apt-get install -y \
    supervisor \
    privoxy \
    sudo \
    jq \
    iproute2 \
    iptables \
    ca-certificates \
    libnss3-tools \
    && rm -rf /var/lib/apt/lists/*

# Install socks5
COPY ./socks5 /usr/local/bin/socks5

# Inject machine ID spoofing script to fix "无法获取设备ID"
RUN apt-get update && apt-get install -y dmidecode && \
    mv /usr/sbin/dmidecode /usr/sbin/dmidecode.bak || true && \
    echo '#!/bin/sh' > /usr/sbin/dmidecode && \
    echo 'echo "83e31294-0989-44fe-eff2-78ac6a0bf2f3"' >> /usr/sbin/dmidecode && \
    chmod +x /usr/sbin/dmidecode

# Extract YunShu package manually
COPY ./YunShu_2.3.10.27_MCD.deb /tmp/yunshu.deb
RUN dpkg-deb -x /tmp/yunshu.deb /tmp/yunshu \
    && cp -r /tmp/yunshu/opt / \
    && cp -r /tmp/yunshu/usr/share/applications /usr/share/ \
    && cp -r /tmp/yunshu/usr/share/icons /usr/share/ \
    && rm -rf /tmp/yunshu /tmp/yunshu.deb \
    && chmod +x /opt/apps/com.eagleyun.yunshu/files/bin/* /opt/apps/com.eagleyun.yunshu/files/yunshu-cross

# Create necessary directories
RUN mkdir -p /opt/apps/com.eagleyun.yunshu/files/bak/ \
    /opt/apps/com.eagleyun.yunshu/files/logs/ \
    /opt/apps/com.eagleyun.yunshu/files/socket/ \
    /opt/apps/com.eagleyun.yunshu/files/tmp/

# Verify libraries
RUN ldd /opt/apps/com.eagleyun.yunshu/files/bin/yunshu-daemon && \
    ldd /opt/apps/com.eagleyun.yunshu/files/yunshu-cross

# Install YunShu scripts
COPY ./src/ubuntu/install/yunshu $INST_SCRIPTS/yunshu/
COPY --from=yunshu-go-builder /out/auto_mfa_daemon /out/get_otp_secret $INST_SCRIPTS/yunshu/
RUN bash $INST_SCRIPTS/yunshu/install_yunshu.sh  && rm -rf $INST_SCRIPTS/yunshu/

COPY ./src/ubuntu/install/yunshu/custom_startup.sh $STARTUPDIR/custom_startup.sh
RUN chmod +x $STARTUPDIR/custom_startup.sh
RUN chmod 755 $STARTUPDIR/custom_startup.sh
######### End Customizations ###########

RUN chown 1000:0 $HOME

ENV HOME /home/kasm-user
WORKDIR $HOME
RUN mkdir -p $HOME && chown -R 1000:0 $HOME

USER root
