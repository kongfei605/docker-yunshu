package yunshu

import (
	"testing"
	"time"
)

func TestTOTPToken(t *testing.T) {
	token, err := TOTPToken("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", time.Unix(59, 0))
	if err != nil {
		t.Fatalf("TOTPToken returned error: %v", err)
	}
	if token != "287082" {
		t.Fatalf("TOTPToken() = %q, want %q", token, "287082")
	}
}

func TestCookieHeaderSortsNames(t *testing.T) {
	header := CookieHeader(map[string]string{
		"z": "last",
		"a": "first",
	})
	if header != "a=first; z=last" {
		t.Fatalf("CookieHeader() = %q", header)
	}
}
