#!/usr/bin/env python3
import time
import sqlite3
import urllib.request
import urllib.parse
import json
import re
import os
import subprocess
import hmac
import base64
import struct
import hashlib

LOG_FILE = "/home/kasm-user/.config/YunshuCross/logs/main.log"
COOKIE_DB = "/home/kasm-user/.config/YunshuCross/Cookies"
CONFIG_FILE = "/home/kasm-user/.config/YunshuCross/config.json"
YUNSHU_CLI = "/opt/apps/com.eagleyun.yunshu/files/bin/yunshu"
MFA_SECRET_ENV = "YUNSHU_MFA_SECRET"
MFA_SECRET_FILE = "/opt/apps/com.eagleyun.yunshu/files/conf/mfa_secret"
SUBMIT_COOLDOWN_SECONDS = 25
SUCCESS_SUPPRESS_SECONDS = 120
SPA_BASE_URL = "https://sp.eagleyun.cn"

last_submit_at = 0
last_success = {}

def log(message):
    print(message, flush=True)

def get_mfa_secret():
    secret = os.environ.get(MFA_SECRET_ENV, "").strip().replace(" ", "")
    if secret:
        return secret

    try:
        with open(MFA_SECRET_FILE, "r", encoding="utf-8") as f:
            return f.read().strip().replace(" ", "")
    except FileNotFoundError:
        log(f"MFA secret is not configured. Set {MFA_SECRET_ENV} or create {MFA_SECRET_FILE}.")
    except Exception as e:
        log(f"Could not read MFA secret: {e}")
    return ""

def get_totp_token(secret):
    key = base64.b32decode(secret, True)
    msg = struct.pack(">Q", int(time.time() // 30))
    h = hmac.new(key, msg, hashlib.sha1).digest()
    o = h[19] & 15
    h = (struct.unpack(">I", h[o:o+4])[0] & 0x7fffffff) % 1000000
    return f"{h:06d}"

def get_cookies():
    cookies = {}
    try:
        conn = sqlite3.connect(COOKIE_DB)
        cursor = conn.cursor()
        cursor.execute("SELECT name, value FROM cookies")
        for row in cursor.fetchall():
            cookies[row[0]] = row[1]
        conn.close()
    except Exception as e:
        log(f"Error reading cookies: {e}")
    return cookies

def submit_mfa(mfa_url):
    global last_submit_at

    now = time.time()
    last_success_at = last_success.get(mfa_url)
    if last_success_at and now - last_success_at < SUCCESS_SUPPRESS_SECONDS:
        log("Skipping MFA submit because this MFA URL was already verified recently.")
        return True

    if now - last_submit_at < SUBMIT_COOLDOWN_SECONDS:
        log("Skipping MFA submit because cooldown is active.")
        return False
    last_submit_at = now

    cookies = get_cookies()
    cookie_str = "; ".join([f"{k}={v}" for k, v in cookies.items()])
    
    req1 = urllib.request.Request(mfa_url, headers={"Cookie": cookie_str})
    try:
        resp1 = urllib.request.urlopen(req1, timeout=15)
        final_url = resp1.geturl()
    except Exception as e:
        log(f"Failed to follow mfa_url: {e}")
        return False
        
    log(f"Redirected to: {final_url}")
    
    parsed = urllib.parse.urlparse(final_url)
    qs = urllib.parse.parse_qs(parsed.query)
    
    payload = {}
    for k, v in qs.items():
        payload[k] = v[0]

    secret = get_mfa_secret()
    if not secret:
        return False

    payload["default_auth_type"] = "OTP"
    try:
        payload["code"] = get_totp_token(secret)
    except Exception as e:
        log(f"Could not generate MFA token: {e}")
        return False
    payload["otp_type"] = "OTP"
    payload["view"] = "browser"
    
    data = urllib.parse.urlencode(payload).encode("utf-8")
    
    verify_url = "https://idp.eagleyun.cn/innerApi/v1/idp/mfaAuth/verifyOTPCode"
    req2 = urllib.request.Request(verify_url, data=data, headers={
        "Cookie": cookie_str,
        "Content-Type": "application/x-www-form-urlencoded",
        "Accept": "application/json",
        "User-Agent": "Mozilla/5.0 (Linux; Android 8.0.0; SM-G955U Build/R16NW) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Mobile Safari/537.36"
    })
    
    try:
        resp2 = urllib.request.urlopen(req2, timeout=15)
        result = resp2.read().decode("utf-8")
        log(f"MFA Verify Result: {result}")
        try:
            parsed_result = json.loads(result)
            if parsed_result.get("is_success") is True:
                if report_mfa_success(parsed_result, cookie_str):
                    connect_private_network()
                last_success[mfa_url] = time.time()
        except json.JSONDecodeError:
            pass
        return True
    except urllib.error.HTTPError as e:
        log(f"MFA Verify Failed: HTTP {e.code} - {e.read().decode('utf-8', errors='replace')}")
        return False
    except Exception as e:
        log(f"MFA Verify Failed: {e}")
        return False

def report_mfa_success(verify_result, cookie_str):
    payload = {
        "ins_id": verify_result.get("ins_id"),
        "te_id": verify_result.get("te_id"),
        "employee_id": verify_result.get("employee_id"),
    }
    data = json.dumps(payload).encode("utf-8")
    callback_url = f"{SPA_BASE_URL}/innerApi/v1/spaController/terminal/ops/mfaAuthSuccess"
    req = urllib.request.Request(callback_url, data=data, headers={
        "Cookie": cookie_str,
        "Content-Type": "application/json",
        "Accept": "application/json",
        "User-Agent": "Mozilla/5.0 (Linux; Android 8.0.0; SM-G955U Build/R16NW) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Mobile Safari/537.36"
    })

    try:
        resp = urllib.request.urlopen(req, timeout=15)
        result = resp.read().decode("utf-8", errors="replace")
        log(f"MFA Success Callback Result: {result}")
        return True
    except urllib.error.HTTPError as e:
        log(f"MFA Success Callback Failed: HTTP {e.code} - {e.read().decode('utf-8', errors='replace')}")
        return False
    except Exception as e:
        log(f"MFA Success Callback Failed: {e}")
        return False

def auto_connect_enabled():
    try:
        with open(CONFIG_FILE, "r", encoding="utf-8") as f:
            config = json.load(f)
        return config.get("systemSetup", {}).get("autoConnect") is True
    except Exception as e:
        log(f"Could not read auto-connect setting: {e}")
        return False

def run_yunshu_cli(args):
    try:
        output = subprocess.check_output(
            [YUNSHU_CLI, *args],
            stderr=subprocess.STDOUT,
            text=True,
            timeout=30,
        )
        return output.strip()
    except subprocess.CalledProcessError as e:
        output = e.output.strip() if e.output else ""
        log(f"YunShu CLI failed ({' '.join(args)}): {output}")
        return ""
    except Exception as e:
        log(f"YunShu CLI failed ({' '.join(args)}): {e}")
        return ""

def connect_private_network():
    if not auto_connect_enabled():
        log("Skipping private network connect because autoConnect is disabled.")
        return False

    status = run_yunshu_cli(["-i"])
    if status:
        log(f"YunShu Status: {status}")
    if "内网已连接" in status:
        log("Private network is already connected.")
        return True

    output = run_yunshu_cli(["-s", "pa"])
    if output:
        log(f"YunShu Connect Result: {output}")
    return "内网已经连接成功" in output or "内网已连接" in output

def open_current_log():
    while True:
        try:
            f = open(LOG_FILE, "r", encoding="utf-8", errors="replace")
            stat = os.fstat(f.fileno())
            f.seek(0, 2)
            log(f"Following {LOG_FILE} inode={stat.st_ino}")
            return f, (stat.st_dev, stat.st_ino)
        except FileNotFoundError:
            time.sleep(2)

def log_was_rotated(identity, f):
    try:
        stat = os.stat(LOG_FILE)
    except FileNotFoundError:
        return True

    if identity != (stat.st_dev, stat.st_ino):
        return True

    try:
        return stat.st_size < f.tell()
    except OSError:
        return True

def follow_log():
    f, identity = open_current_log()

    while True:
        line = f.readline()
        if not line:
            if log_was_rotated(identity, f):
                log("Detected log rotation. Reopening current main.log.")
                f.close()
                f, identity = open_current_log()
            time.sleep(1)
            continue

        if "heartbeat data:" in line and re.search(r"\"is_need_mfa\"\s*:\s*true", line):
            log(f"[{time.ctime()}] Detected MFA Requirement in heartbeat.")
            match = re.search(r"\"mfa_url\":\"(https?://[^\"]+)\"", line)
            if match:
                mfa_url = match.group(1)
                submit_mfa(mfa_url)
            else:
                log("Could not extract mfa_url from heartbeat data.")

if __name__ == "__main__":
    log("Starting Auto MFA Daemon...")
    follow_log()
