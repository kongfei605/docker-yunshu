package main

import (
	"fmt"
	"os"
	"time"

	"github.com/kongfei605/docker-yunshu/yunshu-tools/internal/yunshu"
)

func getBrizooToken() string {
	if _, err := os.Stat(yunshu.CookieDB); err != nil {
		fmt.Printf("Error: Cookie database not found at %s\n", yunshu.CookieDB)
		fmt.Println("Please ensure Yunshu VPN is running and you have logged in.")
		return ""
	}

	token, err := yunshu.GetCookieValue("__Host-brizoo-token")
	if err != nil {
		fmt.Printf("Failed to read sqlite database: %v\n", err)
		return ""
	}
	return token
}

func main() {
	token := getBrizooToken()
	if token == "" {
		fmt.Println("Could not retrieve __Host-brizoo-token cookie.")
		os.Exit(1)
	}

	fmt.Println("Fetching OTP Config from eagleyun.cn...")
	config, err := yunshu.FetchOTPConfig(token, 15*time.Second)
	if err != nil {
		fmt.Printf("Failed to fetch OTP config: %v\n", err)
		fmt.Println("Failed to retrieve OTP config.")
		os.Exit(1)
	}

	fmt.Println("\n=== OTP Configuration ===")
	fmt.Printf("Account Name : %v\n", config.AccountName)
	fmt.Printf("Issuer       : %v\n", config.Issuer)
	fmt.Printf("Period       : %v seconds\n", config.Period)
	fmt.Printf("Digits       : %v\n", config.Digits)
	fmt.Printf("SECRET       : %v\n", config.Secret)
	fmt.Println("=========================")
	fmt.Println()
	fmt.Println("You can use this SECRET to generate 6-digit TOTP codes!")
}
