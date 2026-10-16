package resolvers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
)

// Admin surface for the referral programme, mounted under /api/admin by
// AdminHandler so it inherits the role guard there.
//
//	GET   /api/admin/referral-program        — the offer's settings
//	POST  /api/admin/referral-program        — change them, without a deploy
//	GET   /api/admin/referrers               — who is referring, and how well
//	PATCH /api/admin/referrers/{id}          — UPI id, PAN, notes, block
//	GET   /api/admin/referrals               — rewards owed, with fraud flags
//	GET   /api/admin/referrals/export.csv    — the payout sheet finance works from
//	PATCH /api/admin/referrals/{id}          — approve, reject, or mark paid
//	POST  /api/admin/referrals/bulk          — the same, for a whole filtered page
//
// Every state change here moves money, so every one is written to the audit log
// with the operator's name against it.

// ─── Programme settings ───────────────────────────────────────────────────────

func (h *ReferralHandler) GetProgram(w http.ResponseWriter, r *http.Request) {
	p, err := h.program(r.Context())
	if err != nil {
		h.Log.Error("load referral program failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load the programme settings")
		return
	}
	h.write(w, http.StatusOK, p)
}

func (h *ReferralHandler) UpdateProgram(w http.ResponseWriter, r *http.Request) {
	var p referralProgram
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&p); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch {
	case p.CourseRewardPaise < 0 || p.ExamRewardPaise < 0:
		h.fail(w, http.StatusBadRequest, "rewards cannot be negative")
		return
	case p.FriendDiscountPercent < 0 || p.FriendDiscountPercent > 100:
		h.fail(w, http.StatusBadRequest, "the friend discount must be between 0 and 100 percent")
		return
	case p.AttributionDays < 1 || p.AttributionDays > 365:
		h.fail(w, http.StatusBadRequest, "the attribution window must be between 1 and 365 days")
		return
	}

	if _, err := h.Pool.Exec(r.Context(), `
		UPDATE referral_program SET
			is_active = $1, course_reward_paise = $2, exam_reward_paise = $3,
			friend_discount_percent = $4, friend_discount_cap_paise = $5,
			min_order_paise = $6, monthly_cap_paise = $7, attribution_days = $8,
			terms_url = $9, updated_at = now()
		WHERE id = 1
	`, p.IsActive, p.CourseRewardPaise, p.ExamRewardPaise, p.FriendDiscountPercent,
		p.FriendDiscountCap, p.MinOrderPaise, p.MonthlyCapPaise, p.AttributionDays,
		clip(p.TermsURL, 300)); err != nil {
		h.Log.Error("update referral program failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not save the settings")
		return
	}
	h.audit(r, "referral_program.update", "1", map[string]any{"active": p.IsActive})
	h.write(w, http.StatusOK, map[string]any{"success": true})
}

// ─── Referrers ────────────────────────────────────────────────────────────────

type referrerRow struct {
	ID            string `json:"id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	UpiID         string `json:"upi_id"`
	PAN           string `json:"pan"`
	IsBlocked     bool   `json:"is_blocked"`
	Notes         string `json:"notes"`
	CreatedAt     string `json:"created_at"`
	Clicks        int    `json:"clicks"`
	Conversions   int    `json:"conversions"`
	EarnedRupees  int64  `json:"earned_rupees"`
	PaidRupees    int64  `json:"paid_rupees"`
	PendingRupees int64  `json:"pending_rupees"`
}

func (h *ReferralHandler) ListReferrers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 50
	}

	var where string
	args := []any{}
	if s := strings.TrimSpace(q.Get("search")); s != "" {
		args = append(args, s)
		where = "WHERE (r.name ILIKE '%' || $1 || '%' OR r.email ILIKE '%' || $1 || '%' OR r.code ILIKE '%' || $1 || '%')"
	}

	var total int
	if err := h.Pool.QueryRow(r.Context(), "SELECT count(*) FROM referrers r "+where, args...).Scan(&total); err != nil {
		h.Log.Error("count referrers failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load referrers")
		return
	}

	rows, err := h.Pool.Query(r.Context(), fmt.Sprintf(`
		SELECT r.id::text, r.code, r.name, r.email, r.phone, r.upi_id, r.pan,
		       r.is_blocked, r.notes, r.created_at,
		       (SELECT count(*) FROM referral_clicks c WHERE c.code = r.code),
		       (SELECT count(*) FROM referral_conversions v WHERE v.referrer_id = r.id AND v.status <> 'reversed'),
		       (SELECT COALESCE(sum(reward_paise), 0) FROM referral_conversions v
		         WHERE v.referrer_id = r.id AND v.status IN ('pending', 'approved', 'paid')),
		       (SELECT COALESCE(sum(reward_paise), 0) FROM referral_conversions v
		         WHERE v.referrer_id = r.id AND v.status = 'paid')
		FROM   referrers r %s
		ORDER  BY r.created_at DESC
		LIMIT  $%d OFFSET $%d
	`, where, len(args)+1, len(args)+2), append(args, size, (page-1)*size)...)
	if err != nil {
		h.Log.Error("list referrers failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load referrers")
		return
	}
	defer rows.Close()

	out := []referrerRow{}
	for rows.Next() {
		var x referrerRow
		var created time.Time
		var earned, paid int64
		if err := rows.Scan(&x.ID, &x.Code, &x.Name, &x.Email, &x.Phone, &x.UpiID, &x.PAN,
			&x.IsBlocked, &x.Notes, &created, &x.Clicks, &x.Conversions, &earned, &paid); err != nil {
			h.Log.Error("scan referrer failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not load referrers")
			return
		}
		x.CreatedAt = created.UTC().Format(time.RFC3339)
		x.EarnedRupees, x.PaidRupees, x.PendingRupees = earned/100, paid/100, (earned-paid)/100
		out = append(out, x)
	}
	h.write(w, http.StatusOK, map[string]any{"referrers": out, "total": total, "page": page, "pageSize": size})
}

// UpdateReferrer records payout details and staff notes, and blocks abusers.
func (h *ReferralHandler) UpdateReferrer(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		UpiID     *string `json:"upi_id"`
		PAN       *string `json:"pan"`
		Notes     *string `json:"notes"`
		IsBlocked *bool   `json:"is_blocked"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tag, err := h.Pool.Exec(r.Context(), `
		UPDATE referrers SET
			upi_id     = COALESCE($2, upi_id),
			pan        = COALESCE($3, pan),
			notes      = COALESCE($4, notes),
			is_blocked = COALESCE($5, is_blocked),
			updated_at = now()
		WHERE id = $1::uuid
	`, id, clipPtr(req.UpiID, 120), clipPtr(req.PAN, 20), clipPtr(req.Notes, 2000), req.IsBlocked)
	if err != nil {
		h.Log.Error("update referrer failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not save the referrer")
		return
	}
	if tag.RowsAffected() == 0 {
		h.fail(w, http.StatusNotFound, "referrer not found")
		return
	}
	if req.IsBlocked != nil {
		h.audit(r, "referrer.block", id, map[string]any{"blocked": *req.IsBlocked})
	}
	h.write(w, http.StatusOK, map[string]any{"success": true})
}

// ─── Rewards ──────────────────────────────────────────────────────────────────

type conversionRow struct {
	ID             string `json:"id"`
	Code           string `json:"code"`
	ReferrerName   string `json:"referrer_name"`
	ReferrerEmail  string `json:"referrer_email"`
	ReferrerUPI    string `json:"referrer_upi"`
	FriendName     string `json:"friend_name"`
	FriendEmail    string `json:"friend_email"`
	Kind           string `json:"kind"`
	ItemName       string `json:"item_name"`
	OrderRupees    int64  `json:"order_rupees"`
	DiscountRupees int64  `json:"discount_rupees"`
	RewardRupees   int64  `json:"reward_rupees"`
	Status         string `json:"status"`
	Flags          string `json:"flags"`
	Reason         string `json:"reason"`
	ApprovedBy     string `json:"approved_by"`
	PayoutRef      string `json:"payout_ref"`
	CreatedAt      string `json:"created_at"`
	PaidAt         string `json:"paid_at,omitempty"`
}

const conversionQuery = `
	SELECT v.id::text, v.code, r.name, r.email, r.upi_id, v.friend_name, v.friend_email,
	       v.kind, v.item_name, v.order_paise, v.discount_paise, v.reward_paise,
	       v.status, v.flags, v.reason, v.approved_by, v.payout_ref, v.created_at, v.paid_at
	FROM   referral_conversions v
	JOIN   referrers r ON r.id = v.referrer_id`

func (h *ReferralHandler) scanConversions(rows pgx.Rows) ([]conversionRow, error) {
	out := []conversionRow{}
	for rows.Next() {
		var x conversionRow
		var created time.Time
		var paidAt *time.Time
		var order, discount, reward int64
		if err := rows.Scan(&x.ID, &x.Code, &x.ReferrerName, &x.ReferrerEmail, &x.ReferrerUPI,
			&x.FriendName, &x.FriendEmail, &x.Kind, &x.ItemName, &order, &discount, &reward,
			&x.Status, &x.Flags, &x.Reason, &x.ApprovedBy, &x.PayoutRef, &created, &paidAt); err != nil {
			return nil, err
		}
		x.OrderRupees, x.DiscountRupees, x.RewardRupees = order/100, discount/100, reward/100
		x.CreatedAt = created.UTC().Format(time.RFC3339)
		if paidAt != nil {
			x.PaidAt = paidAt.UTC().Format(time.RFC3339)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func referralFilters(q map[string][]string) (string, []any) {
	get := func(k string) string {
		if v, ok := q[k]; ok && len(v) > 0 {
			return strings.TrimSpace(v[0])
		}
		return ""
	}
	var clauses []string
	var args []any
	if s := get("status"); s != "" {
		args = append(args, s)
		clauses = append(clauses, fmt.Sprintf("v.status = $%d", len(args)))
	}
	if s := get("kind"); s != "" {
		args = append(args, s)
		clauses = append(clauses, fmt.Sprintf("v.kind = $%d", len(args)))
	}
	if s := get("code"); s != "" {
		args = append(args, strings.ToUpper(s))
		clauses = append(clauses, fmt.Sprintf("v.code = $%d", len(args)))
	}
	if get("flagged") == "true" {
		clauses = append(clauses, "v.flags <> ''")
	}
	if s := get("search"); s != "" {
		args = append(args, s)
		clauses = append(clauses, fmt.Sprintf(
			"(r.name ILIKE '%%' || $%d || '%%' OR r.email ILIKE '%%' || $%d || '%%' OR v.friend_email ILIKE '%%' || $%d || '%%')",
			len(args), len(args), len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func (h *ReferralHandler) ListConversions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 50
	}
	where, args := referralFilters(q)

	var total int
	var owed int64
	if err := h.Pool.QueryRow(r.Context(), `
		SELECT count(*), COALESCE(sum(v.reward_paise) FILTER (WHERE v.status IN ('pending','approved')), 0)
		FROM   referral_conversions v JOIN referrers r ON r.id = v.referrer_id `+where, args...).Scan(&total, &owed); err != nil {
		h.Log.Error("count referral conversions failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load rewards")
		return
	}

	rows, err := h.Pool.Query(r.Context(), fmt.Sprintf("%s %s ORDER BY v.created_at DESC LIMIT $%d OFFSET $%d",
		conversionQuery, where, len(args)+1, len(args)+2), append(args, size, (page-1)*size)...)
	if err != nil {
		h.Log.Error("list referral conversions failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load rewards")
		return
	}
	defer rows.Close()
	out, err := h.scanConversions(rows)
	if err != nil {
		h.Log.Error("scan referral conversions failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load rewards")
		return
	}
	h.write(w, http.StatusOK, map[string]any{
		"referrals": out, "total": total, "page": page, "pageSize": size,
		// What finance actually wants to know before opening the screen.
		"owedRupees": owed / 100,
	})
}

// ExportConversions is the payout sheet: one row per reward, with the UPI id to
// pay it to. Paying forty people from a web dialog is how a programme gets
// quietly abandoned.
func (h *ReferralHandler) ExportConversions(w http.ResponseWriter, r *http.Request) {
	where, args := referralFilters(r.URL.Query())
	rows, err := h.Pool.Query(r.Context(), conversionQuery+" "+where+" ORDER BY v.created_at DESC", args...)
	if err != nil {
		h.Log.Error("export referral conversions failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not export rewards")
		return
	}
	defer rows.Close()
	list, err := h.scanConversions(rows)
	if err != nil {
		h.fail(w, http.StatusInternalServerError, "could not export rewards")
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="referral-payouts.csv"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("earned_at,code,referrer,referrer_email,upi_id,friend,friend_email,item,order_rupees,reward_rupees,status,flags,payout_ref\n"))
	for _, x := range list {
		_, _ = fmt.Fprintf(w, "%s,%s,%s,%s,%s,%s,%s,%s,%d,%d,%s,%s,%s\n",
			x.CreatedAt, x.Code, csvCell(x.ReferrerName), csvCell(x.ReferrerEmail), csvCell(x.ReferrerUPI),
			csvCell(x.FriendName), csvCell(x.FriendEmail), csvCell(x.ItemName),
			x.OrderRupees, x.RewardRupees, x.Status, csvCell(x.Flags), csvCell(x.PayoutRef))
	}
}

// UpdateConversion approves, rejects or pays one reward.
func (h *ReferralHandler) UpdateConversion(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Action    string `json:"action"` // approve | reject | pay | reopen
		Reason    string `json:"reason"`
		PayoutRef string `json:"payout_ref"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	who := middleware.UserIDFromContext(r.Context())

	var tag int64
	var err error
	switch req.Action {
	case "approve":
		tag, err = h.exec(r, `
			UPDATE referral_conversions SET status = 'approved', approved_by = $2, approved_at = now(), updated_at = now()
			WHERE id = $1::uuid AND status = 'pending'`, id, who)
	case "reject":
		if strings.TrimSpace(req.Reason) == "" {
			h.fail(w, http.StatusBadRequest, "give a reason — the referrer may ask why")
			return
		}
		tag, err = h.exec(r, `
			UPDATE referral_conversions SET status = 'rejected', reason = $2, approved_by = $3, approved_at = now(), updated_at = now()
			WHERE id = $1::uuid AND status IN ('pending', 'approved')`, id, clip(req.Reason, 300), who)
	case "pay":
		if strings.TrimSpace(req.PayoutRef) == "" {
			h.fail(w, http.StatusBadRequest, "record the UPI reference — it is the only proof this was paid")
			return
		}
		tag, err = h.exec(r, `
			UPDATE referral_conversions SET status = 'paid', payout_ref = $2, paid_at = now(), updated_at = now()
			WHERE id = $1::uuid AND status = 'approved'`, id, clip(req.PayoutRef, 120))
	case "reopen":
		// For a reward rejected in error. A paid one is never reopened here.
		tag, err = h.exec(r, `
			UPDATE referral_conversions SET status = 'pending', reason = '', updated_at = now()
			WHERE id = $1::uuid AND status = 'rejected'`, id)
	default:
		h.fail(w, http.StatusBadRequest, "unknown action")
		return
	}
	if err != nil {
		h.Log.Error("update referral conversion failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not update the reward")
		return
	}
	if tag == 0 {
		h.fail(w, http.StatusConflict, "that reward is not in a state this action can change")
		return
	}
	h.audit(r, "referral."+req.Action, id, map[string]any{"payout_ref": req.PayoutRef, "reason": req.Reason})
	h.write(w, http.StatusOK, map[string]any{"success": true})
}

// BulkUpdate applies one action to a list of rewards — the weekly payout run.
func (h *ReferralHandler) BulkUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs       []string `json:"ids"`
		Action    string   `json:"action"`
		Reason    string   `json:"reason"`
		PayoutRef string   `json:"payout_ref"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.IDs) == 0 {
		h.fail(w, http.StatusBadRequest, "select at least one reward")
		return
	}
	if len(req.IDs) > 500 {
		h.fail(w, http.StatusBadRequest, "too many at once — filter and do it in pages")
		return
	}
	who := middleware.UserIDFromContext(r.Context())

	var sql string
	var args []any
	switch req.Action {
	case "approve":
		sql = `UPDATE referral_conversions SET status = 'approved', approved_by = $2, approved_at = now(), updated_at = now()
		       WHERE id::text = ANY($1) AND status = 'pending'`
		args = []any{req.IDs, who}
	case "reject":
		if strings.TrimSpace(req.Reason) == "" {
			h.fail(w, http.StatusBadRequest, "give a reason")
			return
		}
		sql = `UPDATE referral_conversions SET status = 'rejected', reason = $2, approved_by = $3, approved_at = now(), updated_at = now()
		       WHERE id::text = ANY($1) AND status IN ('pending', 'approved')`
		args = []any{req.IDs, clip(req.Reason, 300), who}
	case "pay":
		if strings.TrimSpace(req.PayoutRef) == "" {
			h.fail(w, http.StatusBadRequest, "record a payout reference for this batch")
			return
		}
		sql = `UPDATE referral_conversions SET status = 'paid', payout_ref = $2, paid_at = now(), updated_at = now()
		       WHERE id::text = ANY($1) AND status = 'approved'`
		args = []any{req.IDs, clip(req.PayoutRef, 120)}
	default:
		h.fail(w, http.StatusBadRequest, "unknown action")
		return
	}

	tag, err := h.Pool.Exec(r.Context(), sql, args...)
	if err != nil {
		h.Log.Error("bulk referral update failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not update the rewards")
		return
	}
	h.audit(r, "referral.bulk_"+req.Action, "", map[string]any{"count": tag.RowsAffected()})
	h.write(w, http.StatusOK, map[string]any{"success": true, "updated": tag.RowsAffected()})
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func (h *ReferralHandler) exec(r *http.Request, sql string, args ...any) (int64, error) {
	tag, err := h.Pool.Exec(r.Context(), sql, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// audit records who did what. Rewards are money, so "the system did it" is
// never an acceptable answer to a question about one.
func (h *ReferralHandler) audit(r *http.Request, action, target string, detail map[string]any) {
	body, _ := json.Marshal(detail)
	if _, err := h.Pool.Exec(r.Context(), `
		INSERT INTO admin_audit_log (actor_id, action, target_id, detail)
		VALUES ($1, $2, $3, $4)
	`, middleware.UserIDFromContext(r.Context()), action, target, string(body)); err != nil {
		h.Log.Warn("referral audit write failed", zap.String("action", action), zap.Error(err))
	}
}

func clipPtr(s *string, max int) *string {
	if s == nil {
		return nil
	}
	v := clip(*s, max)
	return &v
}
