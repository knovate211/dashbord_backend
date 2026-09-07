package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	gqlhandler "github.com/graphql-go/handler"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pkgauth "github.com/knovate211/pkg/auth"
	pkglog "github.com/knovate211/pkg/logger"
	assessmentv1 "github.com/knovate211/proto/assessment/v1"
	"github.com/knovate211/proto/codec"
	executionv1 "github.com/knovate211/proto/execution/v1"
	problemv1 "github.com/knovate211/proto/problem/v1"
	progressv1 "github.com/knovate211/proto/progress/v1"
	submissionv1 "github.com/knovate211/proto/submission/v1"
	userv1 "github.com/knovate211/proto/user/v1"

	"github.com/knovate211/api-gateway/graph/generated"
	"github.com/knovate211/api-gateway/graph/resolvers"
	"github.com/knovate211/api-gateway/middleware"
)

func main() {
	codec.Register()
	cfg := loadConfig()
	log := pkglog.New(cfg.logLevel)
	defer log.Sync() //nolint:errcheck

	// ── Admin DB Pool (shared DATABASE_URL with user-service) ─────────────────
	var adminPool *pgxpool.Pool
	if cfg.databaseURL != "" {
		pool, err := pgxpool.New(context.Background(), cfg.databaseURL)
		if err != nil {
			log.Warn("admin pool init failed — admin endpoints disabled", zap.Error(err))
		} else {
			adminPool = pool
			defer adminPool.Close()
			log.Info("admin DB pool connected")
		}
	}

	// ── User Service gRPC Connection ──────────────────────────────────────────
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	userConn := mustDial(cfg.userServiceAddr, dialOpts, log)
	defer userConn.Close()
	userSvcClient := userv1.NewUserServiceClient(userConn)

	// ── Auth validator ────────────────────────────────────────────────────────
	var jwtValidator *pkgauth.Validator
	if cfg.jwtPublicKey != "" {
		v, err := pkgauth.NewRS256Validator(cfg.jwtPublicKey)
		if err != nil {
			log.Fatal("init RS256 validator", zap.Error(err))
		}
		jwtValidator = v
	} else {
		jwtValidator = pkgauth.NewHMACValidator(cfg.jwtSecret)
	}

	// ── gRPC connections ──────────────────────────────────────────────────────

	probConn := mustDial(cfg.problemServiceAddr, dialOpts, log)
	defer probConn.Close()

	subConn := mustDial(cfg.submissionServiceAddr, dialOpts, log)
	defer subConn.Close()

	execConn := mustDial(cfg.executionServiceAddr, dialOpts, log)
	defer execConn.Close()

	progConn := mustDial(cfg.progressServiceAddr, dialOpts, log)
	defer progConn.Close()

	assessConn := mustDial(cfg.assessmentServiceAddr, dialOpts, log)
	defer assessConn.Close()
	assessmentSvcClient := assessmentv1.NewAssessmentServiceClient(assessConn)

	// ── Build GraphQL schema ──────────────────────────────────────────────────
	clients := &generated.Clients{
		Problems: &resolvers.ProblemClients{
			ProblemSvc: problemv1.NewProblemServiceClient(probConn),
			UserSvc:    userSvcClient,
			Log:        log,
		},
		Submissions: &resolvers.SubmissionClients{
			SubmissionSvc: submissionv1.NewSubmissionServiceClient(subConn),
			ExecutionSvc:  executionv1.NewExecutionServiceClient(execConn),
			Log:           log,
		},
		Progress: &resolvers.ProgressClients{
			ProgressSvc: progressv1.NewProgressServiceClient(progConn),
			Log:         log,
		},
		User: &resolvers.UserClients{
			UserSvc: userSvcClient,
			Log:     log,
		},
		Jobs: resolvers.NewJobClients(),
		Assessments: &resolvers.AssessmentClients{
			AssessmentSvc: assessmentSvcClient,
			Log:           log,
		},
	}

	schema, err := generated.BuildSchema(clients)
	if err != nil {
		log.Fatal("build graphql schema", zap.Error(err))
	}

	// ── HTTP routes ───────────────────────────────────────────────────────────
	gqlHandler := gqlhandler.New(&gqlhandler.Config{
		Schema:     &schema,
		Pretty:     cfg.devMode,
		GraphiQL:   cfg.devMode, // enable playground in dev
		Playground: cfg.devMode,
	})

	mux := http.NewServeMux()

	// GraphQL endpoint
	mux.Handle("/api/graphql", gqlHandler)

	// REST Login endpoint
	mux.HandleFunc("/api/login", handleLogin(userSvcClient, cfg.jwtSecret, log))

	// REST Profile endpoints
	mux.HandleFunc("/api/profile", handleProfile(userSvcClient, log))

	// Admin panel endpoints — require role=admin in JWT
	if adminPool != nil {
		adminHandler := &resolvers.AdminHandler{
			Pool:   adminPool,
			Log:    log,
			Mailer: resolvers.NewUserMailer(cfg.appBaseURL, log),
		}
		mux.Handle("/api/admin/", adminHandler)
		log.Info("admin REST endpoints registered at /api/admin/")

		// Enquiry capture. The POST side is public (see publicPaths below) —
		// visitors filling in a contact form have no account; the admin list
		// side lives under /api/admin/ and stays behind the role guard.
		inquiryHandler, err := resolvers.NewInquiryHandler(context.Background(), adminPool, log)
		if err != nil {
			log.Error("inquiry handler init failed — enquiry capture disabled", zap.Error(err))
		} else {
			mux.Handle("/api/inquiries", inquiryHandler)
			adminHandler.Inquiries = inquiryHandler
			log.Info("enquiry capture registered at /api/inquiries")
		}

		// Scholarship funnel. The apply/claim/config side is public (see
		// publicPaths below) — an applicant has no account yet, which is the
		// whole point of the flow. The application and programme screens live
		// under /api/admin/ and stay behind the role guard.
		//
		// Eligibility is not granted here: apply writes an assessment_invites
		// row, and assessment-service decides on that row alone whether a paper
		// may be started.
		scholarshipHandler, err := resolvers.NewScholarshipHandler(
			context.Background(), adminPool, log, cfg.jwtSecret, cfg.appBaseURL)
		if err != nil {
			log.Error("scholarship handler init failed — scholarship funnel disabled", zap.Error(err))
		} else {
			mux.Handle("/api/scholarship/", scholarshipHandler)
			adminHandler.Scholarships = scholarshipHandler
			// Retires links that lapsed without ever being used, so the admin
			// list distinguishes a candidate about to sit the test from one who
			// walked away weeks ago.
			scholarshipHandler.StartExpirySweeper(context.Background(),
				time.Duration(envInt("SCHOLARSHIP_SWEEP_MIN", 15))*time.Minute)
			log.Info("scholarship funnel registered at /api/scholarship/",
				zap.String("app_base_url", cfg.appBaseURL))
		}

		// Self-service password reset. Public for the same reason as the
		// enquiry form: someone locked out of their account cannot authenticate
		// to ask for a way back in. The handler carries its own rate limiting,
		// single-use expiring codes, and identical responses for known and
		// unknown addresses.
		resetHandler, err := resolvers.NewPasswordResetHandler(context.Background(), adminPool, log)
		if err != nil {
			log.Error("password reset handler init failed — password reset disabled", zap.Error(err))
		} else {
			mux.Handle("/api/password-reset/", resetHandler)
			log.Info("password reset registered at /api/password-reset/")
		}
	}

	// Recruiter portal — test authoring, invitations, results and shortlisting.
	// Role and company-membership checks live inside the handler.
	recruiterHandler := &resolvers.RecruiterHandler{AssessmentSvc: assessmentSvcClient, Log: log}
	mux.Handle("/api/recruiter/", recruiterHandler)
	log.Info("recruiter REST endpoints registered at /api/recruiter/")

	// WebSocket proxy to notification-service
	if cfg.notificationServiceURL != "" {
		wsTarget, err := url.Parse(cfg.notificationServiceURL)
		if err == nil {
			proxy := httputil.NewSingleHostReverseProxy(wsTarget)
			mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
				proxy.ServeHTTP(w, r)
			})
		}
	}

	// Health check
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck
	})

	// ── Apply middleware chain ────────────────────────────────────────────────
	// JWT is parsed for /graphql; resolvers decide per-field auth.
	// /api/inquiries is public: it is the marketing site's contact form.
	// The two password-reset paths are public because someone locked out of
	// their account cannot authenticate to ask for a way back in. They are
	// listed individually rather than by prefix — publicPaths is an exact-match
	// set, and nothing else under that subtree should be reachable unauthenticated.
	// The three scholarship paths are public for the same reason and listed the
	// same way: an applicant has no account until /apply creates one.
	authMW := middleware.Auth(jwtValidator, log,
		"/api/health", "/api/login", "/api/inquiries",
		"/api/password-reset/request", "/api/password-reset/confirm",
		"/api/scholarship/config", "/api/scholarship/apply", "/api/scholarship/claim")
	corsMW := middleware.CORS(cfg.allowedOrigins)

	handler := corsMW(authMW(mux))

	server := &http.Server{
		Addr:         ":" + cfg.httpPort,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Info("api-gateway starting",
		zap.String("port", cfg.httpPort),
		zap.Bool("graphiql", cfg.devMode),
	)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server error", zap.Error(err))
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down api-gateway...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server.Shutdown(ctx) //nolint:errcheck
	log.Info("api-gateway stopped")
}

func mustDial(addr string, opts []grpc.DialOption, log *zap.Logger) *grpc.ClientConn {
	conn, err := grpc.Dial(addr, opts...)
	if err != nil {
		log.Fatal("dial gRPC service", zap.String("addr", addr), zap.Error(err))
	}
	log.Info("gRPC connection established", zap.String("addr", addr))
	return conn
}

type config struct {
	httpPort               string
	problemServiceAddr     string
	submissionServiceAddr  string
	executionServiceAddr   string
	progressServiceAddr    string
	assessmentServiceAddr  string
	userServiceAddr        string
	notificationServiceURL string
	jwtSecret              string
	jwtPublicKey           string
	allowedOrigins         string
	appBaseURL             string // test portal origin, for scholarship claim links
	databaseURL            string // for admin panel direct DB access
	devMode                bool
	logLevel               string
}

func loadConfig() config {
	return config{
		httpPort:               env("HTTP_PORT", "8080"),
		problemServiceAddr:     env("PROBLEM_SERVICE_ADDR", "localhost:50051"),
		submissionServiceAddr:  env("SUBMISSION_SERVICE_ADDR", "localhost:50053"),
		executionServiceAddr:   env("EXECUTION_SERVICE_ADDR", "localhost:50052"),
		progressServiceAddr:    env("PROGRESS_SERVICE_ADDR", "localhost:50054"),
		assessmentServiceAddr:  env("ASSESSMENT_SERVICE_ADDR", "localhost:50056"),
		userServiceAddr:        env("USER_SERVICE_ADDR", "localhost:50055"),
		notificationServiceURL: env("NOTIFICATION_SERVICE_URL", "http://localhost:8081"),
		jwtSecret:              env("JWT_SECRET", "dev-secret-change-in-production"),
		jwtPublicKey:           env("JWT_PUBLIC_KEY", ""),
		allowedOrigins:         env("ALLOWED_ORIGINS", "*"),
		// Where a scholarship applicant is sent to sit their test. Must be the
		// portal's public origin in production — the claim link is built from
		// it, and a wrong value sends applicants nowhere.
		appBaseURL:  env("APP_BASE_URL", "http://localhost:5173"),
		databaseURL: env("DATABASE_URL", ""), // shared with user-service
		devMode:     env("DEV_MODE", "true") == "true",
		logLevel:    env("LOG_LEVEL", "info"),
	}
}

// envInt reads a positive integer setting, falling back when unset or nonsense.
// A zero or negative value would turn a periodic job into a busy loop, so it
// degrades to the default rather than to whatever was typed.
func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ─── Login Handler ──────────────────────────────────────────────

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// ─── Profile Handler ─────────────────────────────────────────────

func handleProfile(userSvc userv1.UserServiceClient, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// OPTIONS (CORS preflight handled by CORS middleware, but handle gracefully)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Authenticated user
		userID := middleware.UserIDFromContext(r.Context())
		if userID == "" {
			http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			resp, err := userSvc.GetProfile(r.Context(), &userv1.GetProfileRequest{UserID: userID})
			if err != nil {
				log.Error("get profile failed", zap.Error(err))
				http.Error(w, `{"error":"failed to get profile"}`, http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(resp.Profile) //nolint:errcheck

		case http.MethodPut:
			var p userv1.UserProfile
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
				return
			}
			p.UserID = userID // enforce user_id from JWT, never from body
			resp, err := userSvc.UpsertProfile(r.Context(), &userv1.UpsertProfileRequest{Profile: &p})
			if err != nil {
				log.Error("upsert profile failed", zap.Error(err))
				http.Error(w, `{"error":"failed to save profile"}`, http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(resp) //nolint:errcheck

		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}

type loginResponse struct {
	Token string      `json:"token"`
	User  profileInfo `json:"user"`
}

type profileInfo struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

func handleLogin(userSvc userv1.UserServiceClient, jwtSecret string, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Handle CORS preflight options
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
			return
		}

		resp, err := userSvc.VerifyUser(r.Context(), &userv1.VerifyUserRequest{
			Email:    req.Email,
			Password: req.Password,
		})
		if err != nil {
			if st, ok := status.FromError(err); ok && st.Code() == codes.Unauthenticated {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"Invalid email or password"}`)) //nolint:errcheck
				return
			}
			// A blank email or password is the caller's mistake, not ours.
			// Reporting it as a 500 sends operators hunting a server fault
			// every time somebody submits an empty form.
			if st, ok := status.FromError(err); ok && st.Code() == codes.InvalidArgument {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"Email and password are required"}`)) //nolint:errcheck
				return
			}
			log.Error("login verification failed", zap.Error(err))
			http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
			return
		}

		// Generate token
		token, err := pkgauth.GenerateToken(resp.Id, resp.Email, resp.Role, jwtSecret, 24*time.Hour)
		if err != nil {
			log.Error("token generation failed", zap.Error(err))
			http.Error(w, `{"error":"Failed to generate auth token"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(loginResponse{
			Token: token,
			User: profileInfo{
				ID:    resp.Id,
				Email: resp.Email,
				Name:  resp.Name,
				Role:  resp.Role,
			},
		}) //nolint:errcheck
	}
}
