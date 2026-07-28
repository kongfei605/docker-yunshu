#!/usr/bin/env bash
    cat /etc/resolv.conf | sed 's/rotate//g' | sed '/^options[[:space:]]*$/d' > /tmp/resolv.conf && cat /tmp/resolv.conf > /etc/resolv.conf
set -ex
START_COMMAND="/opt/apps/com.eagleyun.yunshu/files/yunshu-cross --disable-http2 --no-sandbox"
PGREP="yunshu-cross"
export MAXIMIZE="true"
export MAXIMIZE_NAME="YunShu"
export NODE_ENV=production
MAXIMIZE_SCRIPT=$STARTUPDIR/maximize_window.sh
DEFAULT_ARGS="--no-sandbox"
ARGS=${APP_ARGS:-$DEFAULT_ARGS}

options=$(getopt -o gau: -l go,assign,url: -n "$0" -- "$@") || exit
eval set -- "$options"

while [[ $1 != -- ]]; do
    case $1 in
        -g|--go) GO='true'; shift 1;;
        -a|--assign) ASSIGN='true'; shift 1;;
        -u|--url) OPT_URL=$2; shift 2;;
        *) echo "bad option: $1" >&2; exit 1;;
    esac
done
shift

# Process non-option arguments.
for arg; do
    echo "arg! $arg"
done

FORCE=$2

kasm_exec() {
    if [ -n "$OPT_URL" ] ; then
        URL=$OPT_URL
    elif [ -n "$1" ] ; then
        URL=$1
    fi

    # Since we are execing into a container that already has the browser running from startup,
    #  when we don't have a URL to open we want to do nothing. Otherwise a second browser instance would open.
    if [ -n "$URL" ] ; then
        /usr/bin/filter_ready
        /usr/bin/desktop_ready
        echo "Waiting for yunshu-daemon socket..."
        while [ ! -S /opt/apps/com.eagleyun.yunshu/files/socket/yunshu.socket ]; do sleep 1; done
        echo "Socket found. Waiting 3 extra seconds for daemon to initialize..."
        sleep 3
        bash ${MAXIMIZE_SCRIPT} &
        $START_COMMAND $ARGS $OPT_URL
    else
        echo "No URL specified for exec command. Doing nothing."
    fi
}

kasm_startup() {
    if [ -n "$KASM_URL" ] ; then
        URL=$KASM_URL
    elif [ -z "$URL" ] ; then
        URL=$LAUNCH_URL
    fi

    if [ -z "$DISABLE_CUSTOM_STARTUP" ] ||  [ -n "$FORCE" ] ; then

        echo "Entering process startup loop"
        set +x
        while true
        do
            if ! pgrep -x $PGREP > /dev/null
            then
                /usr/bin/filter_ready
                /usr/bin/desktop_ready
                echo "Waiting for yunshu-daemon socket..."
                while [ ! -S /opt/apps/com.eagleyun.yunshu/files/socket/yunshu.socket ]; do sleep 1; done
                echo "Socket found. Waiting 3 extra seconds for daemon to initialize..."
                sleep 3
                set +e
                pkill -f auto_mfa_daemon.py || true
                nohup python3 /opt/apps/com.eagleyun.yunshu/files/auto_mfa_daemon.py > /opt/apps/com.eagleyun.yunshu/files/logs/auto_mfa.log 2>&1 &
                bash ${MAXIMIZE_SCRIPT} &
                $START_COMMAND $ARGS $URL
                set -e
            fi
            sleep 1
        done
        set -x

    fi

}

# Route and NAT management moved to explicit test phase if needed

/usr/bin/supervisord 2>/dev/null || true

if [ -n "$GO" ] || [ -n "$ASSIGN" ] ; then
    kasm_exec
else
    kasm_startup
fi
