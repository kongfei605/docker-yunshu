#!/usr/bin/env bash
set -ex

sed -i '/^listen-address/d' /etc/privoxy/config
echo 'listen-address 0.0.0.0:8118' >>/etc/privoxy/config

# YunShu daemon service
mkdir -p /var/log/yunshu/
cat <<EOF >/etc/supervisor/conf.d/yunshu.conf
[program:yunshu-daemon]
command=/opt/apps/com.eagleyun.yunshu/files/bin/yunshu-daemon
user=root
autostart=true
autorestart=true
startsecs=5
stopasgroup=true
killasgroup=true
stdout_logfile=/var/log/yunshu/daemon.stdout.log
stderr_logfile=/var/log/yunshu/daemon.stderr.log
priority=10
EOF

# Privoxy
mkdir -p /var/log/privoxy
cat <<EOF >/etc/supervisor/conf.d/privoxy.conf
[program:privoxy]
command=/usr/sbin/privoxy --no-daemon /etc/privoxy/config
autostart=true
autorestart=true
stderr_logfile=/var/log/privoxy/stderr.log
stdout_logfile=/var/log/privoxy/stdout.log
EOF

# Socks5 server
mkdir -p /var/log/socks5/
cat <<EOF >/etc/supervisor/conf.d/socks5.conf
[program:socks5]
command=/usr/local/bin/socks5
autostart=true
autorestart=true
stderr_logfile=/var/log/socks5/stderr.log
stdout_logfile=/var/log/socks5/stdout.log
EOF

# Desktop Shortcut
cp /opt/apps/com.eagleyun.yunshu/entries/applications/com.eagleyun.yunshu.desktop $HOME/Desktop/
chmod +x $HOME/Desktop/com.eagleyun.yunshu.desktop
cp $INST_SCRIPTS/yunshu/*.py /opt/apps/com.eagleyun.yunshu/files/ && chmod +x /opt/apps/com.eagleyun.yunshu/files/*.py
