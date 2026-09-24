package resolvers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func sign(msg, secret string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))
}

func TestValidPaymentSignature(t *testing.T) {
	sig := sign("order_ABC|pay_XYZ", "s3cret")
	if !validPaymentSignature("order_ABC", "pay_XYZ", sig, "s3cret") {
		t.Fatal("valid signature rejected")
	}
	for name, c := range map[string][4]string{
		"wrong secret":  {"order_ABC", "pay_XYZ", sig, "other"},
		"swapped ids":   {"pay_XYZ", "order_ABC", sig, "s3cret"},
		"other payment": {"order_ABC", "pay_OTHER", sig, "s3cret"},
		"empty":         {"order_ABC", "pay_XYZ", "", "s3cret"},
	} {
		if validPaymentSignature(c[0], c[1], c[2], c[3]) {
			t.Errorf("%s: forged signature accepted", name)
		}
	}
}

func TestValidWebhookSignature(t *testing.T) {
	body := []byte(`{"event":"payment.captured"}`)
	if !validWebhookSignature(body, sign(string(body), "wh"), "wh") {
		t.Fatal("valid webhook rejected")
	}
	if validWebhookSignature([]byte(`{"event":"payment.captured","x":1}`), sign(string(body), "wh"), "wh") {
		t.Fatal("tampered body accepted")
	}
}

func TestFeeFor(t *testing.T) {
	cases := []struct {
		course, plan string
		want         int
		ok           bool
	}{
		{"5", "self", 8999, true},
		{"1", "mentor", 14999, true},
		{"testing", "self", 5999, true},
		{"digital-marketing", "mentor", 9999, true},
		{"genai", "self", 11999, true},
		{"3", "self", 4999, true},
		{"5", "gold", 0, false},
		{"nope", "self", 0, false},
	}
	for _, c := range cases {
		got, _, ok := feeFor(c.course, c.plan)
		if got != c.want || ok != c.ok {
			t.Errorf("feeFor(%q,%q) = %d,%v want %d,%v", c.course, c.plan, got, ok, c.want, c.ok)
		}
	}
}

func TestGeneratePassword(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		p, err := generatePassword()
		if err != nil || len(p) != 12 || seen[p] {
			t.Fatalf("bad password %q err=%v", p, err)
		}
		seen[p] = true
	}
}
