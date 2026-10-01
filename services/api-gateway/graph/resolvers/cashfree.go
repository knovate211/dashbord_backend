package resolvers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// cashfree is the payment gateway behind course enrolment and certification
// exams. Both handlers share one client, so they can never disagree about
// keys, environment or API version.
//
// The flow differs from a signature-in-the-browser gateway in one way that
// matters: the browser reports nothing we trust. After checkout it only says
// "I'm done"; the server then asks Cashfree for the order and counts it as paid
// only when Cashfree says PAID for exactly the amount stored on our row.
// Webhooks are signed with the same secret key, so there is no separate
// webhook secret to configure.
//
//	CASHFREE_APP_ID      — x-client-id
//	CASHFREE_SECRET_KEY  — x-client-secret, also the webhook signing key
//	CASHFREE_ENV         — "sandbox" (default) or "production"
//	CASHFREE_API_BASE    — override the API origin (tests)
//	PAYMENT_WEBHOOK_BASE — public https origin of this gateway; when set, each
//	                       order carries its own notify_url
type cashfree struct {
	appID, secret string
	env           string // "sandbox" or "production"
	apiBase       string // ends in /pg
	webhookBase   string
	client        *http.Client
}

// cashfreeAPIVersion is pinned so a Cashfree release cannot change response
// shapes under us.
const cashfreeAPIVersion = "2026-01-01"

func newCashfreeFromEnv() *cashfree {
	env := strings.ToLower(strings.TrimSpace(envOr("CASHFREE_ENV", "sandbox")))
	if env != "production" {
		env = "sandbox"
	}
	base := "https://sandbox.cashfree.com/pg"
	if env == "production" {
		base = "https://api.cashfree.com/pg"
	}
	return &cashfree{
		appID:       strings.TrimSpace(envOr("CASHFREE_APP_ID", "")),
		secret:      strings.TrimSpace(envOr("CASHFREE_SECRET_KEY", "")),
		env:         env,
		apiBase:     strings.TrimRight(envOr("CASHFREE_API_BASE", base), "/"),
		webhookBase: strings.TrimRight(envOr("PAYMENT_WEBHOOK_BASE", ""), "/"),
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *cashfree) enabled() bool    { return c != nil && c.appID != "" && c.secret != "" }
func (c *cashfree) sandbox() bool    { return c.env != "production" }
func (c *cashfree) modeName() string { return c.env }

// notifyURL is the webhook for one order, or "" when no public origin is
// configured (a webhook set on the Cashfree dashboard still works then).
// Cashfree requires https here.
func (c *cashfree) notifyURL(path string) string {
	if !strings.HasPrefix(c.webhookBase, "https://") {
		return ""
	}
	return c.webhookBase + path
}

type cashfreeCustomer struct {
	Name, Email, Phone string
}

type cashfreeOrderReq struct {
	OrderID     string
	AmountPaise int64
	Customer    cashfreeCustomer
	Note        string
	Tags        map[string]string
	ReturnURL   string // optional; https only, except in the sandbox
	NotifyURL   string // optional
}

// createOrder opens an order and returns the payment session id the browser
// SDK needs to show checkout.
func (c *cashfree) createOrder(ctx context.Context, o cashfreeOrderReq) (string, error) {
	meta := map[string]string{}
	// Card payments leave the checkout pop-up for the bank's OTP page and come
	// back through return_url; without one the student is stranded on
	// Cashfree's blank "processing" page. Production takes https only; the
	// sandbox also takes a local http:// site so the flow can be tried.
	if strings.HasPrefix(o.ReturnURL, "https://") || (c.sandbox() && strings.HasPrefix(o.ReturnURL, "http://")) {
		meta["return_url"] = o.ReturnURL
	}
	if o.NotifyURL != "" {
		meta["notify_url"] = o.NotifyURL
	}
	body := map[string]any{
		"order_id":       o.OrderID,
		"order_amount":   float64(o.AmountPaise) / 100,
		"order_currency": "INR",
		"customer_details": map[string]string{
			"customer_id":    cashfreeCustomerID(o.Customer.Email),
			"customer_name":  o.Customer.Name,
			"customer_email": o.Customer.Email,
			"customer_phone": o.Customer.Phone,
		},
		"order_note": clip(o.Note, 200),
		"order_tags": o.Tags,
	}
	if len(meta) > 0 {
		body["order_meta"] = meta
	}
	var out struct {
		PaymentSessionID string `json:"payment_session_id"`
	}
	if err := c.call(ctx, http.MethodPost, "/orders", body, &out); err != nil {
		return "", err
	}
	if out.PaymentSessionID == "" {
		return "", errors.New("cashfree orders: no payment_session_id in response")
	}
	return out.PaymentSessionID, nil
}

// errNotPaid means Cashfree does not (yet) hold a successful payment for the
// order — the student closed checkout, the payment failed, or it is pending.
var errNotPaid = errors.New("order is not paid")

// paidOrder asks Cashfree whether an order is paid. It returns the successful
// payment's id and the amount Cashfree collected, in paise.
func (c *cashfree) paidOrder(ctx context.Context, orderID string) (paymentID string, paidPaise int64, err error) {
	var order struct {
		OrderStatus string  `json:"order_status"`
		OrderAmount float64 `json:"order_amount"`
	}
	if err := c.call(ctx, http.MethodGet, "/orders/"+orderID, nil, &order); err != nil {
		return "", 0, err
	}
	if order.OrderStatus != "PAID" {
		return "", 0, errNotPaid
	}
	var payments []struct {
		CFPaymentID   json.RawMessage `json:"cf_payment_id"`
		PaymentStatus string          `json:"payment_status"`
	}
	if err := c.call(ctx, http.MethodGet, "/orders/"+orderID+"/payments", nil, &payments); err != nil {
		return "", 0, err
	}
	for _, p := range payments {
		if p.PaymentStatus == "SUCCESS" {
			return rawID(p.CFPaymentID), toPaise(order.OrderAmount), nil
		}
	}
	// PAID with no successful payment listed should not happen; treat it as
	// unpaid rather than enrol someone on a guess.
	return "", 0, errNotPaid
}

func (c *cashfree) call(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.apiBase+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("x-client-id", c.appID)
	req.Header.Set("x-client-secret", c.secret)
	req.Header.Set("x-api-version", cashfreeAPIVersion)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		msg := e.Message
		if msg == "" {
			msg = clip(string(raw), 300)
		}
		return fmt.Errorf("cashfree %s %s: HTTP %d: %s", method, strings.SplitN(path, "/", 3)[1], resp.StatusCode, msg)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("cashfree %s: unexpected response", path)
	}
	return nil
}

// cashfreeEvent is the part of a payment webhook the handlers act on.
type cashfreeEvent struct {
	Type      string
	OrderID   string
	PaymentID string
	Status    string
	PaidPaise int64
}

// parseWebhook verifies the signature and decodes a payment webhook.
//
// Cashfree signs base64(HMAC-SHA256(timestamp + raw body, secret key)), sent in
// x-webhook-signature with the timestamp in x-webhook-timestamp. The raw body
// is what is signed, so it must be verified before any parsing.
func (c *cashfree) parseWebhook(r *http.Request) (*cashfreeEvent, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
	if err != nil {
		return nil, errors.New("invalid body")
	}
	if !validCashfreeSignature(raw, r.Header.Get("x-webhook-timestamp"), r.Header.Get("x-webhook-signature"), c.secret) {
		return nil, errBadSignature
	}
	var ev struct {
		Type string `json:"type"`
		Data struct {
			Order struct {
				OrderID     string  `json:"order_id"`
				OrderAmount float64 `json:"order_amount"`
			} `json:"order"`
			Payment struct {
				CFPaymentID   json.RawMessage `json:"cf_payment_id"`
				PaymentStatus string          `json:"payment_status"`
				PaymentAmount float64         `json:"payment_amount"`
			} `json:"payment"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		return nil, errors.New("invalid json")
	}
	return &cashfreeEvent{
		Type:      ev.Type,
		OrderID:   ev.Data.Order.OrderID,
		PaymentID: rawID(ev.Data.Payment.CFPaymentID),
		Status:    ev.Data.Payment.PaymentStatus,
		PaidPaise: toPaise(ev.Data.Payment.PaymentAmount),
	}, nil
}

// succeeded is true for the one event that should grant anything.
func (e *cashfreeEvent) succeeded() bool {
	return e.Type == "PAYMENT_SUCCESS_WEBHOOK" && e.Status == "SUCCESS" && e.OrderID != "" && e.PaymentID != ""
}

var errBadSignature = errors.New("bad signature")

func validCashfreeSignature(body []byte, timestamp, signature, secret string) bool {
	if timestamp == "" || signature == "" || secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write(body)
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(strings.TrimSpace(signature)))
}

// cashfreeCustomerID derives a stable, letters-and-digits-only id from the
// email, which is what Cashfree accepts. Stable so one person's orders group
// together on the Cashfree dashboard.
func cashfreeCustomerID(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(email)))
	return "k" + hex.EncodeToString(sum[:])[:31]
}

// cashfreePhone reduces what was typed to the 10-digit Indian mobile number
// Cashfree requires: spaces, dashes and a +91 / 0 prefix are dropped.
func cashfreePhone(s string) (string, bool) {
	var d strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			d.WriteRune(r)
		}
	}
	n := d.String()
	switch {
	case len(n) == 12 && strings.HasPrefix(n, "91"):
		n = n[2:]
	case len(n) == 11 && strings.HasPrefix(n, "0"):
		n = n[1:]
	}
	if len(n) != 10 || n[0] < '6' {
		return "", false
	}
	return n, true
}

// rawID accepts an id Cashfree sends as either a JSON string or a number.
func rawID(m json.RawMessage) string {
	return strings.Trim(strings.TrimSpace(string(m)), `"`)
}

// toPaise converts a rupee amount (up to two decimals) to paise without
// float truncation (8999.99 * 100 = 899998.99…).
func toPaise(rupees float64) int64 {
	return int64(math.Round(rupees * 100))
}
