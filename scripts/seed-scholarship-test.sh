#!/usr/bin/env bash
# Authors a scholarship paper and points a course at it — step 2 of the
# scholarship smoke test, and Phase 0 of the plan.
#
#   docker compose up -d
#   ./scripts/seed-scholarship-test.sh
#   ./scripts/scholarship-smoke.sh
#
# It is idempotent in the ways that matter: MCQs are only imported when the bank
# is short, and the course-to-paper mapping upserts. It does NOT dedupe
# assessments — running it twice authors a second paper, which is usually what
# you want when iterating on a blueprint. Delete the old one from the admin
# panel, or repoint the course with:
#   curl -X POST $API/api/admin/scholarship-programs …
#
# ─── The blueprint ────────────────────────────────────────────────────────────
# These are defaults, not a product decision — override any of them by env var.
#
# The MCQ section deliberately tests aptitude and programming fundamentals
# rather than course-specific material: candidates are applying to LEARN the
# subject, so examining them on it would select for people who least need the
# scholarship. Swap MCQ_TOPIC once you have course-specific banks.
set -euo pipefail

API=${API:-http://localhost:8080}
ADMIN_EMAIL=${ADMIN_EMAIL:-admin@skillofied.com}
ADMIN_PASS=${ADMIN_PASS:-skillofied123}

TITLE=${TITLE:-"Knovate Scholarship Test"}
COURSE_ID=${COURSE_ID:-5}
COURSE_NAME=${COURSE_NAME:-"Full Stack Development"}

DURATION=${DURATION:-60}          # minutes, whole paper
MCQ_COUNT=${MCQ_COUNT:-20}        # drawn per attempt
MCQ_MARKS=${MCQ_MARKS:-2}         # each  → 40
MCQ_TOPIC=${MCQ_TOPIC:-"Scholarship Aptitude"}
CODING_COUNT=${CODING_COUNT:-2}
CODING_MARKS=${CODING_MARKS:-30}  # each  → 60
PASSING=${PASSING:-40}            # = the lowest award slab, 40/100
NEGATIVE=${NEGATIVE:-0}           # a screen should not punish attempting
TAB_LIMIT=${TAB_LIMIT:-5}

# The paper is sequential: a candidate sees one question at a time and the next
# only unlocks once the current one is answered (a judged submission, for the
# coding half). Backtracking stays on, so a committed answer can still be
# revised — the lock is about not reading ahead, not about finality.

# score% → scholarship%
SLABS=${SLABS:-'[{"minPercent":80,"awardPercent":100},{"minPercent":65,"awardPercent":50},{"minPercent":50,"awardPercent":25}]'}

HERE=$(cd "$(dirname "$0")" && pwd)
MCQ_SEED=${MCQ_SEED:-$HERE/seed-scholarship-mcqs.json}

pass() { printf '  \033[32m✓\033[0m %s\n' "$1"; }
fail() { printf '  \033[31m✗\033[0m %s\n' "$1"; exit 1; }
step() { printf '\n\033[1m%s\033[0m\n' "$1"; }
note() { printf '    \033[2m%s\033[0m\n' "$1"; }

command -v jq >/dev/null || fail "jq is required"

step "1 · admin login"
TOKEN=$(curl -fsS -X POST "$API/api/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASS\"}" | jq -r .token)
[ -n "$TOKEN" ] && [ "$TOKEN" != null ] || fail "could not log in as $ADMIN_EMAIL"
AUTH=(-H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json')
pass "authenticated"

step "2 · MCQ bank"
HAVE=$(curl -fsS "${AUTH[@]}" \
  --get --data-urlencode "topic=$MCQ_TOPIC" --data-urlencode "pageSize=200" \
  "$API/api/recruiter/mcq-bank" | jq '.questions | length')
note "topic \"$MCQ_TOPIC\" currently holds $HAVE question(s)"
if [ "$HAVE" -lt "$MCQ_COUNT" ]; then
  [ -f "$MCQ_SEED" ] || fail "bank is short and $MCQ_SEED is missing"
  IMPORTED=$(curl -fsS -X POST "${AUTH[@]}" "$API/api/recruiter/mcq-bank/import" \
    --data-binary "@$MCQ_SEED" | jq -r '.imported // 0')
  pass "imported $IMPORTED starter question(s)"
  HAVE=$(curl -fsS "${AUTH[@]}" \
    --get --data-urlencode "topic=$MCQ_TOPIC" --data-urlencode "pageSize=200" \
    "$API/api/recruiter/mcq-bank" | jq '.questions | length')
fi
[ "$HAVE" -ge "$MCQ_COUNT" ] || fail "bank has $HAVE questions but the paper draws $MCQ_COUNT"
pass "$HAVE question(s) available to draw from"

step "3 · coding problems"
# Easy first, then Medium. Auto-selection is a convenience for getting a paper
# standing up, NOT a curation decision: whatever sorts first may well be
# language-specific (a question about prototype chains is unanswerable in Go),
# and candidates may sit this in any of five languages. Pin the real set with
#   PROBLEM_IDS="uuid-1,uuid-2" ./scripts/seed-scholarship-test.sh
#
# Titles matching PROBLEM_EXCLUDE are skipped. The seeded bank contains 32
# placeholder problems whose statement is "return the input value directly" and
# whose starter code already does exactly that — submitting the template
# unchanged scores full marks. Two of them sorted first and went into the real
# paper, which made the coding half of a scholarship free.
PROBLEM_EXCLUDE=${PROBLEM_EXCLUDE:-'Prototype|Serialization|placeholder'}
ALL=$(curl -fsS -X POST "${AUTH[@]}" "$API/api/graphql" \
  -d '{"query":"query($pageSize:Int){listProblems(pageSize:$pageSize){problems{id title difficulty}}}","variables":{"pageSize":300}}')
if [ -n "${PROBLEM_IDS:-}" ]; then
  PROBLEMS=$(echo "$ALL" | jq -c --arg ids "$PROBLEM_IDS" \
    '($ids | split(",")) as $want | [.data.listProblems.problems[]? | select(.id | IN($want[]))]')
else
  PROBLEMS=$(echo "$ALL" | jq -c --arg skip "$PROBLEM_EXCLUDE" "
    ([.data.listProblems.problems[]? | select(.difficulty==\"Easy\")] +
     [.data.listProblems.problems[]? | select(.difficulty==\"Medium\")])
    | map(select(.title | test(\$skip; \"i\") | not))
    | .[0:$CODING_COUNT]")
fi
FOUND=$(echo "$PROBLEMS" | jq 'length')
[ "$FOUND" -eq "$CODING_COUNT" ] || fail "need $CODING_COUNT problems, found $FOUND — seed problem-service or check PROBLEM_IDS"
echo "$PROBLEMS" | jq -r '.[] | "    · \(.title) (\(.difficulty))"'
pass "$FOUND problem(s) selected"
[ -n "${PROBLEM_IDS:-}" ] || note "auto-selected — review these before a real campaign"

step "4 · create the paper"
ASSESSMENT=$(curl -fsS -X POST "${AUTH[@]}" "$API/api/recruiter/assessments" -d "{
  \"title\": \"$TITLE — $COURSE_NAME\",
  \"description\": \"Entrance test for the $COURSE_NAME scholarship. Multiple choice plus live coding, auto-graded.\",
  \"purpose\": \"scholarship\",
  \"duration_minutes\": $DURATION,
  \"passing_marks\": $PASSING,
  \"negative_marking\": $NEGATIVE,
  \"shuffle_questions\": true,
  \"shuffle_options\": true,
  \"allow_backtrack\": true,
  \"lock_forward\": true,
  \"reveal_results\": true,
  \"max_attempts\": 1,
  \"proctoring\": {\"require_fullscreen\": true, \"tab_switch_limit\": $TAB_LIMIT, \"block_copy_paste\": true, \"webcam\": false}
}" | jq -r .id)
[ -n "$ASSESSMENT" ] && [ "$ASSESSMENT" != null ] || fail "could not create the assessment"
pass "assessment $ASSESSMENT"
note "purpose=scholarship → invite-only, enforced by resolveInvite"

step "5 · section 1 — multiple choice"
MCQ_SECTION=$(curl -fsS -X POST "${AUTH[@]}" "$API/api/recruiter/assessments/$ASSESSMENT/sections" -d "{
  \"title\": \"Aptitude & Programming Fundamentals\",
  \"kind\": \"mcq\",
  \"order_index\": 0,
  \"pick_count\": $MCQ_COUNT,
  \"pick_topic\": \"$MCQ_TOPIC\",
  \"pick_marks\": $MCQ_MARKS,
  \"partial_credit\": false
}" | jq -r .id)
[ -n "$MCQ_SECTION" ] && [ "$MCQ_SECTION" != null ] || fail "could not create the MCQ section"
pass "$MCQ_COUNT drawn per attempt × $MCQ_MARKS marks = $((MCQ_COUNT * MCQ_MARKS))"
note "drawn fresh per candidate from the bank, so no two papers match"

step "6 · section 2 — coding"
CODE_SECTION=$(curl -fsS -X POST "${AUTH[@]}" "$API/api/recruiter/assessments/$ASSESSMENT/sections" -d "{
  \"title\": \"Coding\",
  \"kind\": \"coding\",
  \"order_index\": 1,
  \"partial_credit\": true
}" | jq -r .id)
[ -n "$CODE_SECTION" ] && [ "$CODE_SECTION" != null ] || fail "could not create the coding section"

QUESTIONS=$(echo "$PROBLEMS" | jq -c "[to_entries[] | {problem_id: .value.id, marks: $CODING_MARKS, order_index: .key}]")
curl -fsS -X PUT "${AUTH[@]}" \
  "$API/api/recruiter/assessments/$ASSESSMENT/sections/$CODE_SECTION/questions" \
  -d "{\"questions\": $QUESTIONS}" >/dev/null || fail "could not attach the coding problems"
pass "$CODING_COUNT problem(s) × $CODING_MARKS marks = $((CODING_COUNT * CODING_MARKS))"

step "7 · publish"
PUB=$(curl -fsS -X POST "${AUTH[@]}" "$API/api/recruiter/assessments/$ASSESSMENT/publish" -d '{"publish":true}')
STATUS=$(echo "$PUB" | jq -r .status)
TOTAL=$(echo "$PUB" | jq -r .total_marks)
[ "$STATUS" = "published" ] || fail "publish failed: $PUB"
pass "published · $TOTAL marks total · pass mark $PASSING"
echo "$PUB" | jq -r '.warnings[]? | "    ! \(.)"'

step "8 · map the course to it"
curl -fsS -X POST "${AUTH[@]}" "$API/api/admin/scholarship-programs" -d "{
  \"course_id\": \"$COURSE_ID\",
  \"course_name\": \"$COURSE_NAME\",
  \"assessment_id\": \"$ASSESSMENT\",
  \"is_active\": true,
  \"seats\": 0,
  \"award_slabs\": $SLABS
}" | jq -e .success >/dev/null || fail "could not map the course"
pass "course $COURSE_ID ($COURSE_NAME) → $ASSESSMENT"
echo "$SLABS" | jq -r '.[] | "    · \(.minPercent)%+ scores earn a \(.awardPercent)% scholarship"'

step "9 · it is live on the public config"
curl -fsS "$API/api/scholarship/config" \
  | jq -e --arg c "$COURSE_ID" '.programs[] | select(.courseId==$c)' >/dev/null \
  || fail "the programme is not showing on /api/scholarship/config"
pass "visible to the marketing site"

printf '\n\033[32mPaper ready.\033[0m  %s marks · %s min · 1 attempt\n' "$TOTAL" "$DURATION"
printf 'Next:  ./scripts/scholarship-smoke.sh\n'
