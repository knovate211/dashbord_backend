# ai.md — project context for AI assistants

Everything an assistant needs to work on `dashbord_backend` without rediscovering it.
Keep this file current when architecture, ports, env vars or workflows change.

---

## 1. What this is

Go microservices backend for the **Skillofide / Knovate** learning platform:
coding practice with a real judge, course quizzes, timed assessments
(scholarship, hiring, certification), admin panel, recruiter portal, live-class
attendance, and paid enrolment via Razorpay.

- Repo: `git@github-knovate211:knovate211/dashbord_backend.git` (Go module paths use `github.com/knovate211/...`)
- Branches: `main` (deploys to prod on push), `stage` (working branch)
- Go workspace: `go.work` (go 1.22) ties together `pkg/`, `proto/` and each `services/*` module. `vendor/` and `third-party/` (Windows redis/nats binaries) are not project code.

### Frontends that call this backend (sibling folders in `../`)

| Folder | What | Dev origin |
|---|---|---|
| `skillofied-app` | Student app / test portal | `http://localhost:5173` |
| `skillofied-admin` | Admin panel | `http://localhost:5174` |
| (skillofied-frontend) | Older frontend | `http://localhost:5176` |
| `knovate-web` | Next.js marketing site: enquiry form, scholarship apply, enrol, certification | `http://localhost:3000` |

Plans and course content live next to them in `../*.md` (`ADMIN_PANEL_PLAN.md`, `SCALING_PLAN.md`, `SCHOLARSHIP_PROGRAM_PLAN.md`, `MARKETING_SITE_PLAN.md`, the `*_COURSE.md` files).

---

## 2. Architecture

```
frontends ──HTTP/GraphQL──► api-gateway :8080 ──gRPC (JSON codec)──► services
                                 │  └── direct Postgres (admin, scholarship, enroll, ...)
                                 └── /ws reverse-proxy ──► notification-service :8081 (host 8085)

execution-service ── Docker socket ──► skillofide/runner-<lang> sandbox containers (NetworkMode none)
services ◄── NATS JetStream (subjects: submission.>, execution.>) ──► notification-service
```

| Service | gRPC port | Owns |
|---|---|---|
| `api-gateway` | HTTP 8080 | REST + GraphQL, JWT auth, CORS, rate limits, emails, Razorpay, JSearch. Also talks to Postgres **directly** (`DATABASE_URL`) for most non-core features |
| `problem-service` | 50051 | problems, test cases, signatures, starter code (Redis cache) |
| `execution-service` | 50052 | judge + sandbox; codegen of drivers/starters per language |
| `submission-service` | 50053 | submissions; orchestrator calls execution, reports to progress |
| `progress-service` | 50054 | per-user problem/set progress (Redis cache) |
| `notification-service` | HTTP 8081 | WebSockets fed by NATS |
| `user-service` | 50055 | users, login check, profiles, courses |
| `assessment-service` | 50056 | assessments, attempts, sections, MCQs, grading, invites, proctoring; sweeper expires attempts |

- gRPC uses a **custom JSON codec** (`proto/codec/codec.go`). The `proto/*/v1/*.go` files are hand-written Go types, not only protoc output.
- Shared code: `pkg/auth` (JWT + bcrypt passwords), `pkg/database` (pgx pool, redis), `pkg/logger` (zap).
- Languages the judge supports: `python javascript java cpp go sql`. The SQL runner boots a throwaway Postgres per test case.

### Layout inside a service
`cmd/main.go` → `internal/{handler,repository,cache,...}` → `migrations/NNN_*.up/down.sql`

### api-gateway specifics (`services/api-gateway/`)
- `cmd/main.go` wires every route. `middleware/auth.go` does JWT validation. The `publicPaths` list is **exact-match**, so every public route is listed on its own.
- `graph/schema.graphqls` + `graph/generated/` + `graph/resolvers/*.go`.
- Most features are plain REST handlers living in `graph/resolvers/` (despite the folder name):
  `admin*.go, inquiry*.go, scholarship*.go, hiring*.go, integrity.go, similarity.go, attendance.go, enroll.go, certification*.go, password_reset*.go, recruiter.go, welcome_notify.go, job.resolvers.go`.
- If `DATABASE_URL` is unset, all the direct-DB handlers are **silently disabled** at startup. Check the logs for `... disabled`.

---

## 3. HTTP surface (gateway)

| Route | Auth | Purpose |
|---|---|---|
| `POST /api/login` | public | returns JWT + user (note: README shows `/login`; the real path is `/api/login`) |
| `/api/profile` | JWT | profile get/update |
| `/api/graphql` | JWT | problems, submissions, run/submit code, scratchpad, quizzes, assessments/attempts, proctor events, job search. GraphiQL is on when `DEV_MODE=true` |
| `/api/admin/*` | role=admin | dashboard, users (bulk import, emails login details), audit log, problems authoring, tests, inquiries, scholarships, attendance schedules, enrol orders, certifications |
| `/api/inquiries` | POST public | contact-form lead capture |
| `/api/scholarship/{config,apply,claim}` | public | scholarship funnel; apply writes `assessment_invites` |
| `/api/hiring/claim` | public | candidate magic link; the rest of `/api/hiring/*` checks company membership |
| `/api/integrity/*` | JWT | device sessions, code snapshots, reviewer report (similarity, AI signals, risk rating) |
| `/api/attendance/*` | JWT | live classes and attendance |
| `/api/enroll/{config,order,verify,webhook}` | public | Razorpay course purchase |
| `/api/certification/{config,order,verify,webhook,claim,credential}` | public | certification exams + public credential verify |
| `/api/password-reset/{request,confirm}` | public | self-service reset with single-use codes |
| `/api/recruiter/*` | JWT + role/company | test authoring, invites, results, shortlists |
| `/ws` | — | proxied to notification-service |
| `/api/health` | public | `{"status":"ok"}` |

JWT claims: `user_id`, `email`, `role` (`admin | student | instructor`, plus recruiter/company membership via `company_members`).

---

## 4. Data

Single Postgres DB `skillofide`. Each service migrates with its own table name:
`schema_migrations_{problem,submission,progress,assessment}` (the `db-migrate` compose service runs them all).

Main tables: `users, user_profiles, user_courses, problems, problem_signatures, reference_solutions, test_cases, starter_codes, examples, hints, problem_constraints, problem_tags, practice_sets, submissions, problem_progress, set_progress, user_progress, problem_user_status, quiz_keys, user_quiz_attempts, assessments, assessment_sections, section_questions, mcq_questions, mcq_options, attempts, attempt_questions, attempt_sessions, attempt_submissions, assessment_invites, proctor_events, code_snapshots, scholarship_programs, scholarship_applications, certification_exams, certification_registrations, certificates, companies, company_members, hiring_candidates, shortlists, shortlist_entries, enrollment_orders, inquiries, class_schedules, class_attendance, password_resets, admin_audit_log, activity_log`.

Some gateway tables (such as `admin_audit_log`) are created at startup by the handler (`EnsureAuditTable`, `New*Handler`), not by migrations.

---

## 5. Running locally

```bash
docker compose up --build -d          # postgres, redis, nats, mailpit, migrations, all services
./scripts/build-runners.sh            # REQUIRED: judge sandbox images for all 6 languages
```

| Thing | Host address |
|---|---|
| Gateway | `http://localhost:8080` |
| Postgres | `localhost:5432`, user/pass/db `skillofide` / `password` / `skillofide` |
| Redis | `localhost:6380`, **not 6379**, because another local project (`b4igo-redis`) holds 6379. Containers still use `redis:6379` |
| NATS | `4222` (client), `8222` (monitoring) |
| Mailpit inbox | `http://localhost:8025` (SMTP 1025). All dev email lands here |
| notification-service | `8085` → container 8081 (avoids the React Native Metro port 8081) |

- DB shell: `docker exec -it dashbord_backend-postgres-1 psql -U skillofide -d skillofide`
- Add/update a user over gRPC: `go run add-user.go <email> "<name>" <password> <role>`
- Dev admin: `admin@skillofied.com` / `skillofied123` (local only)
- Rebuild a single service without restarting dependencies: `docker compose up -d --build --no-deps api-gateway`
- Windows helpers: `start-dev.ps1`, `start-dev-local.ps1`

A missing runner image fails **silently as far as the learner can tell**: correct code shows up as a RuntimeError (`No such image: skillofide/runner-<lang>`).

---

## 6. Configuration (key env vars)

Gateway: `JWT_SECRET`, `ALLOWED_ORIGINS`, `DATABASE_URL`, `DEV_MODE`, `APP_BASE_URL` (test-portal origin; builds every claim link), `SITE_BASE_URL`,
`SMTP_HOST/PORT/USER/PASS/FROM` (leaving `SMTP_HOST` unset is supported: links are only logged), `INQUIRY_NOTIFY_TO`, `SCHOLARSHIP_NOTIFY_TO`, `CERTIFICATION_NOTIFY_TO`,
`RAZORPAY_KEY_ID/KEY_SECRET/WEBHOOK_SECRET` (empty = online payment off, "request a callback" instead), `JSEARCH_API_KEY`,
rate limits `{SCHOLARSHIP,INQUIRY,ENROLL,PASSWORD_RESET}_{IP,EMAIL}_LIMIT` (per-IP defaults to 0/off because campuses share one NAT IP), `SCHOLARSHIP_SWEEP_MIN`, `CERTIFICATION_SWEEP_MIN`.

Execution: `EXEC_STARTUP_GRACE_MS` (default 3000), `EXEC_MAX_CONCURRENT` (blank = derived from CPU count; oversubscribing makes correct code time out).

Service addresses: `*_SERVICE_ADDR`, `NOTIFICATION_SERVICE_URL`, `POSTGRES_DSN`, `REDIS_ADDR`, `NATS_URL`, `GRPC_PORT`, `LOG_LEVEL`.

`.env` holds real secrets. Never commit it or copy values out of it. `.env.example` is the template.

---

## 7. Deploy & CI

- `.github/workflows/deploy-backend.yml`: push to `main` → build all 8 service images + 6 runner images → push to ECR (`skillofide-<name>`) → SSH to EC2 → `docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d`, then retag runners as `skillofide/runner-<lang>:latest`.
- Prod uses **Amazon RDS** Postgres (`ap-south-1`). Local postgres is scaled to 0 replicas. Redis/NATS are not exposed publicly.
- `.github/workflows/validate-problems.yml`: on changes to codegen/judge/sandbox/problem migrations/seeds, runs unit tests and then a content gate. Every reference solution must pass, and every generated starter must **fail**, so no answers leak into starters.
- `deployments/k8s/` contains partial, older k8s manifests that are not in use.

---

## 8. Practice-problem "execution contract"

Problems declare an entry point, typed params, return type and comparison mode (`problem_signatures`, `problems.io_mode`, class-mode columns). The starter code and the judge driver are **both generated** from that declaration (`execution-service/internal/codegen/`), so they can't disagree.
Deploy order matters: migrations → runner images → seed signatures → new binaries. See `PRACTICE_RUNBOOK.md`.
Tools: `services/execution-service/cmd/{validate-problems,regen-starters,infer-signatures,augment-testcases,driver-conformance}`, `scripts/validate-problems.sh`, `scripts/seed-signatures.sql`, `scripts/backfill-io-modes.sql`. Large seed data: `seed-problems.go`.

---

## 9. Other docs in this repo

| File | Covers |
|---|---|
| `README.md` | Quick setup, DB commands, add-user, login test |
| `PRACTICE_RUNBOOK.md` | Deploying the execution contract safely |
| `SCHOLARSHIP_RUNBOOK.md` | Email setup (Mailpit/Gmail/SES) and end-to-end scholarship checks (`scripts/scholarship-*.{sh,mjs}`) |
| `PROXY_PLAN.md` | Planned Caddy reverse proxy + Squid forward proxy. Phase 1 fixes `clientIP()` trusting a spoofable `X-Forwarded-For` (`graph/resolvers/inquiry.go`) |

---

## 10. Gotchas & conventions

- Build or test in Go: `go build ./services/... ./proto/...`, `go test ./services/api-gateway/... ./pkg/...`.
- Adding a public route means adding its exact path to `publicPaths` in the gateway `main.go`.
- Adding a migration means creating the next `NNN_name.up.sql` + `.down.sql` in that service's `migrations/`. Prod picks it up on deploy.
- Scholarship eligibility is decided by assessment-service from the `assessment_invites` row alone, not by the gateway.
- Scholarship applicants are not students and never see their own score.
- The execution sandbox must stay `NetworkMode: "none"`.
- Code comments explain **why** (see the compose file). Match that style.
- Work in progress (uncommitted as of 2026-09-25): certification exams (`certification*.go`, assessment migration `006_certification_purpose`), prod compose + deploy workflow edits.
