package yunshu

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"database/sql"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	CookieDB      = "/home/kasm-user/.config/YunshuCross/Cookies"
	ConfigFile    = "/home/kasm-user/.config/YunshuCross/config.json"
	YunshuCLI     = "/opt/apps/com.eagleyun.yunshu/files/bin/yunshu"
	MFASecretEnv  = "YUNSHU_MFA_SECRET"
	MFASecretFile = "/opt/apps/com.eagleyun.yunshu/files/conf/mfa_secret"
	SPABaseURL    = "https://sp.eagleyun.cn"
	OTPConfigURL  = SPABaseURL + "/innerApi/v1/spaController/terminal/ops/getOTPConfig"
	UserAgent     = "Mozilla/5.0 (Linux; Android 8.0.0; SM-G955U Build/R16NW) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Mobile Safari/537.36"
)

type OTPConfig struct {
	AccountName string `json:"accountName"`
	Issuer      string `json:"issuer"`
	Period      any    `json:"period"`
	Digits      any    `json:"digits"`
	Secret      string `json:"secret"`
}

type apiResponse struct {
	Code    int             `json:"code"`
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func OpenCookieDB(readOnly bool) (*sql.DB, error) {
	if _, err := os.Stat(CookieDB); err != nil {
		return nil, err
	}

	mode := "rw"
	if readOnly {
		mode = "ro"
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=%s&_pragma=busy_timeout(5000)", CookieDB, mode))
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func GetCookieValue(name string) (string, error) {
	db, err := OpenCookieDB(true)
	if err != nil {
		return "", err
	}
	defer db.Close()

	var value string
	if err := db.QueryRow("SELECT value FROM cookies WHERE name = ?", name).Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

func GetAllCookies() (map[string]string, error) {
	db, err := OpenCookieDB(true)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query("SELECT name, value FROM cookies")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cookies := make(map[string]string)
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		cookies[name] = value
	}
	return cookies, rows.Err()
}

func CookieHeader(cookies map[string]string) string {
	names := make([]string, 0, len(cookies))
	for name := range cookies {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+cookies[name])
	}
	return strings.Join(parts, "; ")
}

func FetchOTPConfig(token string, timeout time.Duration) (*OTPConfig, error) {
	req, err := http.NewRequest(http.MethodGet, OTPConfigURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", "__Host-brizoo-token="+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d - %s", resp.StatusCode, string(body))
	}

	var api apiResponse
	if err := json.Unmarshal(body, &api); err != nil {
		return nil, err
	}
	if api.Code != 200 || !api.Success {
		return nil, fmt.Errorf("code=%d message=%s", api.Code, api.Message)
	}

	var config OTPConfig
	if len(api.Data) > 0 && !bytes.Equal(api.Data, []byte("null")) {
		if err := json.Unmarshal(api.Data, &config); err != nil {
			return nil, err
		}
	}
	return &config, nil
}

func GetMFASecret(logMissing bool, logf func(string, ...any)) string {
	secret := strings.ReplaceAll(strings.TrimSpace(os.Getenv(MFASecretEnv)), " ", "")
	if secret != "" {
		return secret
	}

	content, err := os.ReadFile(MFASecretFile)
	if err == nil {
		return strings.ReplaceAll(strings.TrimSpace(string(content)), " ", "")
	}
	if errors.Is(err, os.ErrNotExist) {
		if logMissing {
			logf("MFA secret is not configured. Set %s or create %s.", MFASecretEnv, MFASecretFile)
		}
		return ""
	}
	logf("Could not read MFA secret: %v", err)
	return ""
}

func WriteMFASecret(secret string) error {
	secret = strings.ReplaceAll(strings.TrimSpace(secret), " ", "")
	if _, err := TOTPToken(secret, time.Now()); err != nil {
		return fmt.Errorf("fetched OTP secret is invalid: %w", err)
	}

	secretDir := filepath.Dir(MFASecretFile)
	if err := os.MkdirAll(secretDir, 0700); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(secretDir, ".mfa_secret.")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.WriteString(secret + "\n"); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, MFASecretFile); err != nil {
		return err
	}
	if err := os.Chmod(MFASecretFile, 0600); err != nil {
		return err
	}
	ok = true
	return nil
}

func TOTPToken(secret string, now time.Time) (string, error) {
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	if normalized == "" {
		return "", errors.New("empty secret")
	}
	if remainder := len(normalized) % 8; remainder != 0 {
		normalized += strings.Repeat("=", 8-remainder)
	}

	key, err := base32.StdEncoding.DecodeString(normalized)
	if err != nil {
		return "", err
	}

	msg := make([]byte, 8)
	binary.BigEndian.PutUint64(msg, uint64(now.Unix()/30))

	hash := hmac.New(sha1.New, key)
	hash.Write(msg)
	sum := hash.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1000000), nil
}
