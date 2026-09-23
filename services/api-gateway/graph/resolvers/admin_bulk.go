package resolvers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

// Filter facets, bulk actions and export for the enquiry and scholarship
// screens. They live on AdminHandler, not on the enquiry/scholarship handlers,
// so they sit behind the admin role guard and land in the audit log.

// maxBulk caps one bulk request. "Delete all matching" beyond this is refused
// rather than silently truncated, so the count the admin confirmed is the
// count that happens.
const maxBulk = 5000

// bulkRequest selects rows either by explicit ids or by "everything matching
// these filters". Filters use the same query-string keys as the list endpoint
// and go through the same WHERE builder, so the rows acted on are exactly the
// rows the admin was looking at.
type bulkRequest struct {
	IDs         []string          `json:"ids"`
	AllMatching bool              `json:"all_matching"`
	Filters     map[string]string `json:"filters"`
	// Expected is the count the admin saw and confirmed. For all_matching it
	// must still match, so a row arriving between confirm and submit is not
	// deleted unseen.
	Expected int    `json:"expected"`
	Action   string `json:"action"`
	Status   string `json:"status"`
}

func (b bulkRequest) values() url.Values {
	v := url.Values{}
	for k, s := range b.Filters {
		v.Set(k, s)
	}
	return v
}

type facet struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Count int    `json:"count"`
}

func (h *AdminHandler) facetQuery(ctx context.Context, sql string, args ...any) []facet {
	out := []facet{}
	rows, err := h.Pool.Query(ctx, sql, args...)
	if err != nil {
		h.Log.Warn("facet query failed", zap.Error(err))
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var f facet
		if rows.Scan(&f.Value, &f.Label, &f.Count) == nil {
			out = append(out, f)
		}
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
// Enquiries
// ─────────────────────────────────────────────────────────────────────────────

// GET /api/admin/inquiries/facets — the values each filter can take, with
// counts, so dropdowns only offer options that match something.
func (h *AdminHandler) handleInquiryFacets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h.json(w, http.StatusOK, map[string]any{
		"status":   h.facetQuery(ctx, `SELECT status, '', COUNT(*)::int FROM inquiries GROUP BY status ORDER BY 3 DESC`),
		"source":   h.facetQuery(ctx, `SELECT source, '', COUNT(*)::int FROM inquiries GROUP BY source ORDER BY 3 DESC`),
		"interest": h.facetQuery(ctx, `SELECT interest, '', COUNT(*)::int FROM inquiries WHERE interest <> '' GROUP BY interest ORDER BY 3 DESC`),
	})
}

// POST /api/admin/inquiries/bulk  action: status | delete
func (h *AdminHandler) handleInquiryBulk(w http.ResponseWriter, r *http.Request) {
	var req bulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch req.Action {
	case "delete":
	case "status":
		switch req.Status {
		case "new", "contacted", "closed", "spam":
		default:
			h.jsonErr(w, http.StatusBadRequest, "status must be new, contacted, closed or spam")
			return
		}
	default:
		h.jsonErr(w, http.StatusBadRequest, "action must be status or delete")
		return
	}

	ctx := r.Context()
	var ids []string
	if req.AllMatching {
		where, args := inquiryWhere(req.values())
		var err error
		if ids, err = h.collectIDs(ctx, `SELECT id::text FROM inquiries `+where, args...); err != nil {
			h.Log.Error("resolve enquiry filter failed", zap.Error(err))
			h.jsonErr(w, http.StatusInternalServerError, "bulk action failed")
			return
		}
	} else {
		ids = req.IDs
	}
	if msg := checkBulkSize(req, len(ids)); msg != "" {
		h.jsonErr(w, http.StatusBadRequest, msg)
		return
	}

	var sql string
	args := []any{ids}
	if req.Action == "delete" {
		sql = `DELETE FROM inquiries WHERE id::text = ANY($1) RETURNING email`
	} else {
		sql = `UPDATE inquiries SET status = $2, updated_at = now() WHERE id::text = ANY($1) RETURNING email`
		args = append(args, req.Status)
	}
	emails, err := h.collectIDs(ctx, sql, args...)
	if err != nil {
		h.Log.Error("enquiry bulk action failed", zap.String("action", req.Action), zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "bulk action failed")
		return
	}

	action := "enquiries.deleted"
	detail := map[string]any{"count": len(emails), "all_matching": req.AllMatching}
	if req.Action == "status" {
		action = "enquiries.status_changed"
		detail["status"] = req.Status
	}
	if req.AllMatching {
		detail["filters"] = req.Filters
	}
	if len(emails) > 0 {
		h.audit(ctx, action, "", "", detail)
	}
	h.json(w, http.StatusOK, map[string]int{"affected": len(emails), "requested": len(ids)})
}

// GET /api/admin/inquiries/export.csv — same filters as the table.
func (h *AdminHandler) handleInquiryExport(w http.ResponseWriter, r *http.Request) {
	where, args := inquiryWhere(r.URL.Query())
	rows, err := h.Pool.Query(r.Context(), `
		SELECT name, email, phone, whatsapp, interest, source, status, message, notes, created_at
		FROM inquiries `+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		h.Log.Error("export enquiries failed", zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "failed to export enquiries")
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="enquiries.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"name", "email", "phone", "whatsapp", "interest", "source", "status", "message", "notes", "received"})
	n := 0
	for rows.Next() {
		var c [9]string
		var at time.Time
		if rows.Scan(&c[0], &c[1], &c[2], &c[3], &c[4], &c[5], &c[6], &c[7], &c[8], &at) != nil {
			continue
		}
		rec := make([]string, 0, 10)
		for _, v := range c {
			rec = append(rec, csvSafe(v))
		}
		_ = cw.Write(append(rec, at.Format(time.RFC3339)))
		n++
	}
	cw.Flush()
	h.audit(r.Context(), "enquiries.exported", "", "", map[string]any{"rows": n})
}

// ─────────────────────────────────────────────────────────────────────────────
// Scholarship applications
// ─────────────────────────────────────────────────────────────────────────────

// GET /api/admin/scholarships/facets — derived statuses and courses, with counts.
func (h *AdminHandler) handleScholarshipFacets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	base, _, _ := applicationQuery(url.Values{})
	h.json(w, http.StatusOK, map[string]any{
		"status": h.facetQuery(ctx, base+`SELECT status, '', COUNT(*)::int FROM app GROUP BY status ORDER BY 3 DESC`),
		"course": h.facetQuery(ctx, base+`SELECT course_id, MIN(course_name), COUNT(*)::int FROM app GROUP BY course_id ORDER BY 3 DESC`),
	})
}

// POST /api/admin/scholarships/bulk  action: delete
//
// Each application is removed with the same cleanup as a single delete
// (invite, attempts, the applicant account when unused), all in one
// transaction: either every selected application goes or none do.
func (h *AdminHandler) handleScholarshipBulk(w http.ResponseWriter, r *http.Request) {
	var req bulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Action != "delete" {
		h.jsonErr(w, http.StatusBadRequest, "action must be delete")
		return
	}

	ctx := r.Context()
	var ids []string
	if req.AllMatching {
		base, where, args := applicationQuery(req.values())
		var err error
		if ids, err = h.collectIDs(ctx, base+`SELECT id::text FROM app `+where, args...); err != nil {
			h.Log.Error("resolve scholarship filter failed", zap.Error(err))
			h.jsonErr(w, http.StatusInternalServerError, "bulk delete failed")
			return
		}
	} else {
		ids = req.IDs
	}
	if msg := checkBulkSize(req, len(ids)); msg != "" {
		h.jsonErr(w, http.StatusBadRequest, msg)
		return
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		h.jsonErr(w, http.StatusInternalServerError, "bulk delete failed")
		return
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	deleted, accounts := 0, 0
	for _, id := range ids {
		_, removed, err := h.Scholarships.deleteApplicationTx(ctx, tx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue // already gone — not a reason to abandon the rest
		}
		if err != nil {
			h.Log.Error("bulk delete application failed", zap.String("id", id), zap.Error(err))
			h.jsonErr(w, http.StatusInternalServerError, "bulk delete failed — nothing was deleted")
			return
		}
		deleted++
		if removed {
			accounts++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		h.Log.Error("commit bulk delete failed", zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "bulk delete failed — nothing was deleted")
		return
	}

	detail := map[string]any{"count": deleted, "accounts_removed": accounts, "all_matching": req.AllMatching}
	if req.AllMatching {
		detail["filters"] = req.Filters
	}
	if deleted > 0 {
		h.audit(ctx, "scholarships.deleted", "", "", detail)
	}
	h.json(w, http.StatusOK, map[string]int{"affected": deleted, "requested": len(ids), "accounts_removed": accounts})
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func (h *AdminHandler) collectIDs(ctx context.Context, sql string, args ...any) ([]string, error) {
	rows, err := h.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// checkBulkSize returns a user-facing error, or "" when the request may proceed.
func checkBulkSize(req bulkRequest, n int) string {
	switch {
	case n == 0:
		return "nothing selected"
	case n > maxBulk:
		return fmt.Sprintf("at most %d rows per bulk action — narrow the filters", maxBulk)
	case req.AllMatching && req.Expected != n:
		return fmt.Sprintf("the list changed since you confirmed (%d now match, not %d) — review and try again", n, req.Expected)
	}
	return ""
}
