#!/usr/bin/env python3
import sqlite3
import json
import urllib.request
import os

COOKIE_DB = "/home/kasm-user/.config/YunshuCross/Cookies"
API_URL = "https://sp.eagleyun.cn/innerApi/v1/spaController/terminal/ops/getOTPConfig"

def get_brizoo_token():
    if not os.path.exists(COOKIE_DB):
        print(f"Error: Cookie database not found at {COOKIE_DB}")
        print("Please ensure Yunshu VPN is running and you have logged in.")
        return None

    try:
        conn = sqlite3.connect(COOKIE_DB)
        cursor = conn.cursor()
        cursor.execute("SELECT value FROM cookies WHERE name = \"__Host-brizoo-token\"")
        result = cursor.fetchone()
        conn.close()
        if result:
            return result[0]
    except Exception as e:
        print(f"Failed to read sqlite database: {e}")
    return None

def fetch_otp_config(token):
    req = urllib.request.Request(API_URL)
    req.add_header("Cookie", f"__Host-brizoo-token={token}")
    try:
        response = urllib.request.urlopen(req)
        data = json.loads(response.read().decode("utf-8"))
        if data.get("code") == 200 and data.get("success"):
            return data.get("data", {})
        else:
            print("API returned an error or unsuccessful status.")
            print(data)
    except Exception as e:
        print(f"Failed to fetch OTP config: {e}")
    return None

if __name__ == "__main__":
    token = get_brizoo_token()
    if not token:
        print("Could not retrieve __Host-brizoo-token cookie.")
        exit(1)

    print("Fetching OTP Config from eagleyun.cn...")
    config = fetch_otp_config(token)
    
    if config:
        print("\n=== OTP Configuration ===")
        print(f"Account Name : {config.get(\"accountName\")}")
        print(f"Issuer       : {config.get(\"issuer\")}")
        print(f"Period       : {config.get(\"period\")} seconds")
        print(f"Digits       : {config.get(\"digits\")}")
        print(f"SECRET       : {config.get(\"secret\")}")
        print("=========================\n")
        print("You can use this SECRET to generate 6-digit TOTP codes!")
    else:
        print("Failed to retrieve OTP config.")
