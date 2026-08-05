package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/kongfei605/docker-yunshu/yunshu-tools/internal/yunshu"
)

const (
	logFile               = "/home/kasm-user/.config/YunshuCross/logs/main.log"
	submitCooldown        = 25 * time.Second
	successSuppress       = 120 * time.Second
	secretRefreshInterval = 300 * time.Second
	secretRefreshRetry    = 30 * time.Second
	connectCheckInterval  = 60 * time.Second
	verifyURL             = "https://idp.eagleyun.cn/innerApi/v1/idp/mfaAuth/verifyOTPCode"
	mfaSuccessCallbackURL = yunshu.SPABaseURL + "/innerApi/v1/spaController/terminal/ops/mfaAuthSuccess"
)

var (
	lastSubmitAt        time.Time
	lastSecretRefreshAt time.Time
	lastConnectCheckAt  time.Time
	lastSuccess         = map[string]time.Time{}
	needMFARegexp       = regexp.MustCompile(`"is_need_mfa"\s*:\s*true`)
	authedRegexp        = regexp.MustCompile(`"is_authed"\s*:\s*true`)
	mfaURLRegexp        = regexp.MustCompile(`"mfa_url":"(https?://[^"]+)"`)
)

func logf(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

func refreshMFASecret(force bool) bool {
	if strings.TrimSpace(os.Getenv(yunshu.MFASecretEnv)) != "" {
		return true
	}

	now := time.Now()
	if !force && now.Sub(lastSecretRefreshAt) < secretRefreshInterval {
		return yunshu.GetMFASecret(false, logf) != ""
	}

	lastSecretRefreshAt = now
	token, err := yunshu.GetCookieValue("__Host-brizoo-token")
	if err != nil {
		logf("Error reading cookie __Host-brizoo-token: %v", err)
		lastSecretRefreshAt = now.Add(-secretRefreshInterval + secretRefreshRetry)
		return false
	}
	if token == "" {
		lastSecretRefreshAt = now.Add(-secretRefreshInterval + secretRefreshRetry)
		return false
	}

	config, err := yunshu.FetchOTPConfig(token, 15*time.Second)
	if err != nil {
		logf("Failed to fetch OTP config: %v", err)
		lastSecretRefreshAt = now.Add(-secretRefreshInterval + secretRefreshRetry)
		return false
	}
	secret := strings.ReplaceAll(strings.TrimSpace(config.Secret), " ", "")
	if secret == "" {
		logf("OTP config did not include a secret.")
		lastSecretRefreshAt = now.Add(-secretRefreshInterval + secretRefreshRetry)
		return false
	}

	current := yunshu.GetMFASecret(false, logf)
	if current == secret {
		return true
	}

	if err := yunshu.WriteMFASecret(secret); err != nil {
		logf("Could not write MFA secret: %v", err)
		return false
	}
	logf("MFA secret refreshed from OTP config.")
	return true
}

func submitMFA(mfaURL string) bool {
	now := time.Now()
	if lastSuccessAt, ok := lastSuccess[mfaURL]; ok && now.Sub(lastSuccessAt) < successSuppress {
		logf("Skipping MFA submit because this MFA URL was already verified recently.")
		return true
	}

	if now.Sub(lastSubmitAt) < submitCooldown {
		logf("Skipping MFA submit because cooldown is active.")
		return false
	}
	lastSubmitAt = now

	cookies, err := yunshu.GetAllCookies()
	if err != nil {
		logf("Error reading cookies: %v", err)
		return false
	}
	cookieHeader := yunshu.CookieHeader(cookies)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, mfaURL, nil)
	if err != nil {
		logf("Failed to create mfa_url request: %v", err)
		return false
	}
	req.Header.Set("Cookie", cookieHeader)

	resp, err := client.Do(req)
	if err != nil {
		logf("Failed to follow mfa_url: %v", err)
		return false
	}
	finalURL := resp.Request.URL.String()
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	logf("Redirected to: %s", finalURL)

	parsedURL, err := url.Parse(finalURL)
	if err != nil {
		logf("Failed to parse redirected URL: %v", err)
		return false
	}
	payload := url.Values{}
	for key, values := range parsedURL.Query() {
		if len(values) > 0 {
			payload.Set(key, values[0])
		}
	}

	refreshMFASecret(yunshu.GetMFASecret(false, logf) == "")
	secret := yunshu.GetMFASecret(true, logf)
	if secret == "" {
		return false
	}

	token, err := yunshu.TOTPToken(secret, time.Now())
	if err != nil {
		logf("Could not generate MFA token: %v", err)
		return false
	}

	payload.Set("default_auth_type", "OTP")
	payload.Set("code", token)
	payload.Set("otp_type", "OTP")
	payload.Set("view", "browser")

	req, err = http.NewRequest(http.MethodPost, verifyURL, strings.NewReader(payload.Encode()))
	if err != nil {
		logf("Failed to create MFA verify request: %v", err)
		return false
	}
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", yunshu.UserAgent)

	resp, err = client.Do(req)
	if err != nil {
		logf("MFA Verify Failed: %v", err)
		return false
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		logf("MFA Verify Failed: %v", readErr)
		return false
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logf("MFA Verify Failed: HTTP %d - %s", resp.StatusCode, string(body))
		return false
	}

	result := string(body)
	logf("MFA Verify Result: %s", result)

	var parsedResult map[string]any
	if err := json.Unmarshal(body, &parsedResult); err == nil {
		if parsedResult["is_success"] == true {
			if reportMFASuccess(parsedResult, cookieHeader) {
				connectPrivateNetwork()
			}
			lastSuccess[mfaURL] = time.Now()
		}
	}
	return true
}

func reportMFASuccess(verifyResult map[string]any, cookieHeader string) bool {
	payload := map[string]any{
		"ins_id":      verifyResult["ins_id"],
		"te_id":       verifyResult["te_id"],
		"employee_id": verifyResult["employee_id"],
	}
	data, err := json.Marshal(payload)
	if err != nil {
		logf("MFA Success Callback Failed: %v", err)
		return false
	}

	req, err := http.NewRequest(http.MethodPost, mfaSuccessCallbackURL, bytes.NewReader(data))
	if err != nil {
		logf("MFA Success Callback Failed: %v", err)
		return false
	}
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", yunshu.UserAgent)

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		logf("MFA Success Callback Failed: %v", err)
		return false
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		logf("MFA Success Callback Failed: %v", readErr)
		return false
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logf("MFA Success Callback Failed: HTTP %d - %s", resp.StatusCode, string(body))
		return false
	}

	logf("MFA Success Callback Result: %s", string(body))
	return true
}

func autoConnectEnabled() bool {
	content, err := os.ReadFile(yunshu.ConfigFile)
	if err != nil {
		logf("Could not read auto-connect setting: %v", err)
		return false
	}

	var config struct {
		SystemSetup struct {
			AutoConnect bool `json:"autoConnect"`
		} `json:"systemSetup"`
	}
	if err := json.Unmarshal(content, &config); err != nil {
		logf("Could not read auto-connect setting: %v", err)
		return false
	}
	return config.SystemSetup.AutoConnect
}

func runYunshuCLI(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, yunshu.YunshuCLI, args...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		logf("YunShu CLI failed (%s): %s", strings.Join(args, " "), textOrError(text, err))
		return ""
	}
	return text
}

func textOrError(text string, err error) string {
	if text != "" {
		return text
	}
	return err.Error()
}

func connectPrivateNetwork() bool {
	if !autoConnectEnabled() {
		logf("Skipping private network connect because autoConnect is disabled.")
		return false
	}

	status := runYunshuCLI("-i")
	if status != "" {
		logf("YunShu Status: %s", status)
	}
	if strings.Contains(status, "内网已连接") {
		logf("Private network is already connected.")
		return true
	}

	output := runYunshuCLI("-s", "pa")
	if output != "" {
		logf("YunShu Connect Result: %s", output)
	}
	return strings.Contains(output, "内网已经连接成功") || strings.Contains(output, "内网已连接")
}

func maybeConnectPrivateNetwork() {
	now := time.Now()
	if now.Sub(lastConnectCheckAt) < connectCheckInterval {
		return
	}
	lastConnectCheckAt = now
	connectPrivateNetwork()
}

func openCurrentLog() (*os.File, os.FileInfo) {
	for {
		file, err := os.Open(logFile)
		if err == nil {
			info, statErr := file.Stat()
			if statErr == nil {
				_, _ = file.Seek(0, io.SeekEnd)
				logf("Following %s inode=%d", logFile, inode(info))
				return file, info
			}
			_ = file.Close()
			logf("Could not stat %s: %v", logFile, statErr)
		} else if !os.IsNotExist(err) {
			logf("Could not open %s: %v", logFile, err)
		}
		time.Sleep(2 * time.Second)
	}
}

func inode(info os.FileInfo) uint64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Ino
	}
	return 0
}

func logWasRotated(identity os.FileInfo, file *os.File) bool {
	currentInfo, err := os.Stat(logFile)
	if err != nil {
		return true
	}
	if !os.SameFile(identity, currentInfo) {
		return true
	}
	offset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return true
	}
	return currentInfo.Size() < offset
}

func followLog() {
	file, identity := openCurrentLog()
	reader := bufio.NewReader(file)

	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			handleLogLine(line)
		}
		if err == nil {
			continue
		}
		if err != io.EOF {
			logf("Error reading log: %v", err)
		}
		if logWasRotated(identity, file) {
			logf("Detected log rotation. Reopening current main.log.")
			_ = file.Close()
			file, identity = openCurrentLog()
			reader = bufio.NewReader(file)
		}
		refreshMFASecret(false)
		time.Sleep(1 * time.Second)
	}
}

func handleLogLine(line string) {
	if !strings.Contains(line, "heartbeat data:") {
		return
	}

	if !needMFARegexp.MatchString(line) {
		if authedRegexp.MatchString(line) {
			maybeConnectPrivateNetwork()
		}
		return
	}

	logf("[%s] Detected MFA Requirement in heartbeat.", time.Now().Format(time.ANSIC))
	match := mfaURLRegexp.FindStringSubmatch(line)
	if len(match) < 2 {
		logf("Could not extract mfa_url from heartbeat data.")
		return
	}
	submitMFA(match[1])
}

func main() {
	logf("Starting Auto MFA Daemon...")
	followLog()
}
