package resolvers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func signCashfree(ts string, body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestCashfreeWebhookSignature(t *testing.T) {
	c := &cashfree{appID: "app", secret: "s3cret"}
	body := []byte(`{"type":"PAYMENT_SUCCESS_WEBHOOK","data":{"order":{"order_id":"enr_1","order_amount":8999.99},"payment":{"cf_payment_id":1453995084705707520,"payment_status":"SUCCESS","payment_amount":8999.99}}}`)
	ts := "1758091049000"

	send := func(body []byte, ts, sig string) (*cashfreeEvent, error) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
		req.Header.Set("x-webhook-timestamp", ts)
		req.Header.Set("x-webhook-signature", sig)
		return c.parseWebhook(req)
	}

	ev, err := send(body, ts, signCashfree(ts, body, "s3cret"))
	if err != nil {
		t.Fatalf("valid signature refused: %v", err)
	}
	if !ev.succeeded() || ev.OrderID != "enr_1" || ev.PaymentID != "1453995084705707520" || ev.PaidPaise != 899999 {
		t.Fatalf("decoded %+v", ev)
	}

	tampered := []byte(strings.Replace(string(body), "8999.99", "1.00", 2))
	if _, err := send(tampered, ts, signCashfree(ts, body, "s3cret")); !errors.Is(err, errBadSignature) {
		t.Fatalf("a changed amount must break the signature, got %v", err)
	}
	if _, err := send(body, "1758091049001", signCashfree(ts, body, "s3cret")); !errors.Is(err, errBadSignature) {
		t.Fatalf("the timestamp is part of what is signed, got %v", err)
	}
	if _, err := send(body, ts, signCashfree(ts, body, "other")); !errors.Is(err, errBadSignature) {
		t.Fatalf("another account's secret must not verify, got %v", err)
	}
	if _, err := send(body, "", ""); !errors.Is(err, errBadSignature) {
		t.Fatalf("missing headers must be refused, got %v", err)
	}
}

func TestCashfreePhone(t *testing.T) {
	for in, want := range map[string]string{
		"9876543210":       "9876543210",
		"+91 98765 43210":  "9876543210",
		"919876543210":     "9876543210",
		"09876543210":      "9876543210",
		"98765-43210":      "9876543210",
		"12345":            "",
		"5876543210":       "", // Indian mobiles start 6–9
		"+1 415 555 01234": "",
	} {
		got, ok := cashfreePhone(in)
		if ok != (want != "") || got != want {
			t.Errorf("cashfreePhone(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

func TestCashfreeCustomerIDIsAlphanumericAndStable(t *testing.T) {
	a, b := cashfreeCustomerID("Ana@Example.com"), cashfreeCustomerID("ana@example.com")
	if a != b {
		t.Fatal("the same person must map to one customer whatever the email's case")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9]{3,50}$`).MatchString(a) {
		t.Fatalf("customer id %q breaks Cashfree's rule (alphanumeric, 3–50)", a)
	}
}

func TestToPaiseDoesNotTruncate(t *testing.T) {
	for rupees, want := range map[float64]int64{8999: 899900, 8999.99: 899999, 0.29: 29, 4999.5: 499950} {
		if got := toPaise(rupees); got != want {
			t.Errorf("toPaise(%v) = %d, want %d", rupees, got, want)
		}
	}
}

// fakeCashfree answers the three calls the handlers make.
func fakeCashfree(t *testing.T, orderStatus string, payments string) (*cashfree, *map[string]any) {
	t.Helper()
	created := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-client-id") != "app" || r.Header.Get("x-client-secret") != "sec" || r.Header.Get("x-api-version") != cashfreeAPIVersion {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"authentication Failed"}`))
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/pg/orders":
			_ = json.NewDecoder(r.Body).Decode(&created)
			_, _ = w.Write([]byte(`{"order_id":"enr_1","payment_session_id":"session_abc","order_status":"ACTIVE"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/pg/orders/enr_1":
			_, _ = w.Write([]byte(`{"order_id":"enr_1","order_status":"` + orderStatus + `","order_amount":14999}`))
		case r.Method == http.MethodGet && r.URL.Path == "/pg/orders/enr_1/payments":
			_, _ = w.Write([]byte(payments))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return &cashfree{appID: "app", secret: "sec", env: "sandbox", apiBase: srv.URL + "/pg", client: srv.Client()}, &created
}

func TestCashfreeCreateOrder(t *testing.T) {
	c, created := fakeCashfree(t, "ACTIVE", `[]`)
	sid, err := c.createOrder(context.Background(), cashfreeOrderReq{
		OrderID: "enr_1", AmountPaise: 1499950,
		Customer:  cashfreeCustomer{Name: "Ana", Email: "ana@example.com", Phone: "9876543210"},
		ReturnURL: "http://localhost:3000/enroll", // fine in the sandbox
	})
	if err != nil || sid != "session_abc" {
		t.Fatalf("createOrder = %q, %v", sid, err)
	}
	got := *created
	if got["order_amount"] != 14999.5 || got["order_currency"] != "INR" || got["order_id"] != "enr_1" {
		t.Fatalf("order body %v", got)
	}
	if cust := got["customer_details"].(map[string]any); cust["customer_phone"] != "9876543210" {
		t.Fatalf("customer %v", cust)
	}
	if meta, _ := got["order_meta"].(map[string]any); meta["return_url"] != "http://localhost:3000/enroll" {
		t.Fatalf("the sandbox must get a local return_url, or card payments strand the student; sent %v", got["order_meta"])
	}

	c.env = "production"
	for k := range *created {
		delete(*created, k)
	}
	if _, err := c.createOrder(context.Background(), cashfreeOrderReq{OrderID: "enr_1", AmountPaise: 100,
		Customer: cashfreeCustomer{Phone: "9876543210"}, ReturnURL: "http://site.test/enroll"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := (*created)["order_meta"]; ok {
		t.Fatalf("production must not send an http return_url, sent %v", (*created)["order_meta"])
	}
	c.env = "sandbox"

	c.secret = "wrong"
	if _, err := c.createOrder(context.Background(), cashfreeOrderReq{OrderID: "enr_1", AmountPaise: 100}); err == nil ||
		!strings.Contains(err.Error(), "authentication Failed") {
		t.Fatalf("an auth failure must surface Cashfree's message, got %v", err)
	}
}

func TestCashfreePaidOrder(t *testing.T) {
	c, _ := fakeCashfree(t, "PAID",
		`[{"cf_payment_id":111,"payment_status":"FAILED"},{"cf_payment_id":"222","payment_status":"SUCCESS"}]`)
	pid, paise, err := c.paidOrder(context.Background(), "enr_1")
	if err != nil || pid != "222" || paise != 1499900 {
		t.Fatalf("paidOrder = %q, %d, %v", pid, paise, err)
	}

	c, _ = fakeCashfree(t, "ACTIVE", `[]`)
	if _, _, err := c.paidOrder(context.Background(), "enr_1"); !errors.Is(err, errNotPaid) {
		t.Fatalf("an unpaid order must report errNotPaid, got %v", err)
	}

	c, _ = fakeCashfree(t, "PAID", `[{"cf_payment_id":1,"payment_status":"PENDING"}]`)
	if _, _, err := c.paidOrder(context.Background(), "enr_1"); !errors.Is(err, errNotPaid) {
		t.Fatalf("PAID without a successful payment must not count, got %v", err)
	}
}
