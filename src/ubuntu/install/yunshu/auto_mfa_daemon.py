#!/usr/bin/env python3
import time
import sqlite3
import urllib.request
import urllib.parse
import json
import re
import os
import hmac
import base64
import struct
import hashlib

LOG_FILE = "/home/kasm-user/.config/YunshuCross/logs/main.log"
COOKIE_DB = "/home/kasm-user/.config/YunshuCross/Cookies"
SECRET = "24VA64YGXND26PULCUKGYY2BPFZJ3H6B"

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
        print(f"Error reading cookies: {e}")
    return cookies

def submit_mfa(mfa_url):
    cookies = get_cookies()
    cookie_str = "; ".join([f"{k}={v}" for k, v in cookies.items()])
    
    req1 = urllib.request.Request(mfa_url, headers={"Cookie": cookie_str})
    try:
        resp1 = urllib.request.urlopen(req1)
        final_url = resp1.geturl()
    except Exception as e:
        print(f"Failed to follow mfa_url: {e}")
        return False
        
    print(f"Redirected to: {final_url}")
    
    parsed = urllib.parse.urlparse(final_url)
    qs = urllib.parse.parse_qs(parsed.query)
    
    payload = {}
    for k, v in qs.items():
        payload[k] = v[0]
        
    payload["default_auth_type"] = "OTP"
    payload["code"] = get_totp_token(SECRET)
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
        resp2 = urllib.request.urlopen(req2)
        result = resp2.read().decode("utf-8")
        print(f"MFA Verify Result: {result}")
        os.system("pkill yunshu-cross")
        return True
    except urllib.error.HTTPError as e:
        print(f"MFA Verify Failed: HTTP {e.code} - {e.read().decode(utf-8)}")
        return False
    except Exception as e:
        print(f"MFA Verify Failed: {e}")
        return False

def follow_log():
    while not os.path.exists(LOG_FILE):
        time.sleep(2)
        
    with open(LOG_FILE, "r") as f:
        f.seek(0, 2)
        while True:
            line = f.readline()
            if not line:
                time.sleep(1)
                continue
            
            if "heartbeat data:" in line and "\"is_need_mfa\":true" in line:
                print(f"[{time.ctime()}] Detected MFA Requirement in heartbeat.")
                match = re.search(r"\"mfa_url\":\"(https?://[^\"]+)\"", line)
                if match:
                    mfa_url = match.group(1)
                    submit_mfa(mfa_url)
                else:
                    print("Could not extract mfa_url from heartbeat data.")

if __name__ == "__main__":
    print("Starting Auto MFA Daemon...")
    follow_log()
