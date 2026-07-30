package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"tessst/middleware"
	"tessst/service"
	"tessst/storage"
)

// ── Template helper ─────────────────────────────────────────────
// Parses an HTML file from the current directory and serves it.
func serveTemplate(w http.ResponseWriter, file string, data interface{}) {
	tmpl, err := template.ParseFiles(file)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), 500)
		return
	}
	if err := tmpl.Execute(w, data); err != nil {
		slog.Error("template execute failed", "error", err)
	}
}

// ── Context keys (local — not exported; middleware/ has its own)

type contextKey string

func (k contextKey) String() string { return "taskflow." + string(k) }

// requestIDKey stores a request-scoped *slog.Logger in the context.
// The raw ID string is stored separately via middleware.ContextWithRequestID
// so middleware/x402.go can read it without importing this package.
const requestIDKey = contextKey("request_id")

// ── Middleware ──────────────────────────────────────────────────

// requestID is a middleware that ensures every request has a unique
// identifier. It first checks for an incoming X-Request-Id header
// (from a load balancer / proxy), falling back to a randomly generated
// hex string. The ID and a request-scoped logger are stored in the
// request context so all handlers can include them in their log lines.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			// Generate a random 8-byte hex string (16 chars).
			var buf [8]byte
			if _, err := rand.Read(buf[:]); err != nil {
				// Fallback: extremely unlikely to fail on a healthy system.
				// Use a timestamp to ensure uniqueness across concurrent calls.
				id = fmt.Sprintf("%x", time.Now().UnixNano())
			} else {
				id = hex.EncodeToString(buf[:])
			}
		}

		// Attach the ID to the response so callers can correlate.
		w.Header().Set("X-Request-Id", id)

		// Store the ID in context so downstream middleware (including
		// x402) can read it via middleware.RequestIDFromContext.
		ctx := middleware.ContextWithRequestID(r.Context(), id)

		// Also store a request-scoped logger with the ID pre-attached.
		logger := slog.Default().With("request_id", id)
		ctx = context.WithValue(ctx, requestIDKey, logger)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// reqLog returns a logger pre-configured with the request's ID
// (if any), or the default logger if no request-scoped logger
// is found in the context.
func reqLog(r *http.Request) *slog.Logger {
	if logger, ok := r.Context().Value(requestIDKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

// logging wraps any handler and logs each incoming request with
// the request ID (from context) and basic request metadata.
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqLog(r).Info("request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// Stub auth guard — replace with real session check.
func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: read session cookie, validate, redirect to /login if missing
		next(w, r)
	}
}

// ── Handlers (HTML / Web UI) ───────────────────────────────────

// GET /login   — show login form
// POST /login  — validate credentials, set session cookie
func loginHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		serveTemplate(w, "login.html", nil)
	case http.MethodPost:
		email := r.FormValue("email")
		_ = r.FormValue("password") // TODO: validate with bcrypt.CompareHashAndPassword
		reqLog(r).Info("login attempt", "email", email)
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// GET /register   — show registration form
// POST /register  — create user, redirect to /login
func registerHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		serveTemplate(w, "register.html", nil)
	case http.MethodPost:
		name := r.FormValue("name")
		email := r.FormValue("email")
		password := r.FormValue("password")
		confirm := r.FormValue("password_confirm")
		reqLog(r).Info("register", "name", name, "email", email)
		if password != confirm {
			http.Error(w, "Passwords do not match", http.StatusBadRequest)
			return
		}
		// TODO: bcrypt.GenerateFromPassword(password), insert into DB
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// POST /logout — clear session cookie, redirect /login
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "session_token", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// GET /dashboard
func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	serveTemplate(w, "dashboard.html", nil)
}

// GET  /tasks            — list all tasks (HTML)
// POST /tasks            — create task (HTML form POST)
// GET  /tasks/{id}       — task detail
// POST /tasks/{id}       — update task
// POST /tasks/{id}/delete
// POST /tasks/{id}/attachments — file upload
//
// Note: API-style POST /api/tasks and GET /api/projects/{id}/tasks
// are protected by the x402 payment middleware (see main()).
func tasksHandler(w http.ResponseWriter, r *http.Request) {
	// Strip leading /tasks
	path := strings.TrimPrefix(r.URL.Path, "/tasks")
	path = strings.TrimPrefix(path, "/")

	parts := strings.SplitN(path, "/", 3)
	id := ""
	if len(parts) > 0 {
		id = parts[0]
	}
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}

	switch {
	// /tasks or /tasks/
	case id == "":
		if r.Method == http.MethodPost {
			title := r.FormValue("title")
			reqLog(r).Info("create task", "title", title)
			// TODO: INSERT into DB
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		} else {
			serveTemplate(w, "dashboard.html", nil)
		}

	// /tasks/{id}/delete
	case sub == "delete":
		reqLog(r).Info("delete task", "task_id", id)
		// TODO: DELETE FROM tasks WHERE id = ? AND user_id = ?
		http.Redirect(w, r, "/tasks", http.StatusSeeOther)

	// /tasks/{id}/attachments
	case sub == "attachments":
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			files := r.MultipartForm.File["attachment"]
			for _, fh := range files {
				reqLog(r).Info("file uploaded", "filename", fh.Filename, "size", fh.Size, "task_id", id)
				// TODO: save file, record in DB
			}
			http.Redirect(w, r, "/tasks/"+id, http.StatusSeeOther)
		}

	// /tasks/{id}
	default:
		if r.Method == http.MethodPost {
			reqLog(r).Info("update task", "task_id", id, "title", r.FormValue("title"))
			// TODO: UPDATE tasks SET ... WHERE id = ?
			http.Redirect(w, r, "/tasks/"+id, http.StatusSeeOther)
		} else {
			serveTemplate(w, "task-detail.html", nil)
		}
	}
}

// GET /upload   — show upload page
// POST /upload  — handle general file upload
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		serveTemplate(w, "upload.html", nil)
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		files := r.MultipartForm.File["files"]
		for _, fh := range files {
			reqLog(r).Info("upload", "filename", fh.Filename, "size", fh.Size)
			// TODO: os.MkdirAll("./uploads", 0755)
		}
		http.Redirect(w, r, "/upload", http.StatusSeeOther)
	}
}

// GET/POST /profile and sub-routes
func profileHandler(w http.ResponseWriter, r *http.Request) {
	serveTemplate(w, "profile.html", nil)
}

// ── API Handlers (JSON, x402-protected) ────────────────────────

// apiHandlers groups the API handler dependencies so we can inject
// them via a constructor rather than package-level globals.
type apiHandlers struct {
	svc *service.TaskService
}

// newAPIHandlers creates the handler struct with all dependencies.
func newAPIHandlers(svc *service.TaskService) *apiHandlers {
	return &apiHandlers{svc: svc}
}

// createTask is the x402-protected API handler for POST /api/tasks.
// External tools / AI agents call this with a PAYMENT-SIGNATURE header.
func (h *apiHandlers) createTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		ProjectID   string   `json:"project_id"`
		Assignees   []string `json:"assignees"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "bad_request", "invalid JSON body", http.StatusBadRequest)
		return
	}

	task, err := h.svc.CreateTask(r.Context(), req.Title, req.Description, req.ProjectID, req.Assignees)
	if err != nil {
		writeJSONError(w, "creation_failed", err.Error(), http.StatusUnprocessableEntity)
		return
	}

	reqLog(r).Info("api task created", "task_id", task.ID, "title", task.Title, "project_id", task.ProjectID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

// getProjectTasks is the x402-protected API handler for
// GET /api/projects/{id}/tasks. Returns all tasks for a project.
func (h *apiHandlers) getProjectTasks(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	reqLog(r).Info("api get project tasks", "project_id", projectID)

	tasks, err := h.svc.GetProjectTasks(r.Context(), projectID)
	if err != nil {
		writeJSONError(w, "internal_error", err.Error(), http.StatusInternalServerError)
		return
	}

	// Return an empty array instead of null when there are no tasks.
	if tasks == nil {
		tasks = []storage.Task{}
	}

	resp := map[string]interface{}{
		"ok":         true,
		"project_id": projectID,
		"tasks":      tasks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// updateTask is the x402-protected API handler for
// PUT /api/tasks/{id}. Updates an existing task.
func (h *apiHandlers) updateTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	var req struct {
		Title       string   `json:"title,omitempty"`
		Description string   `json:"description,omitempty"`
		Status      string   `json:"status,omitempty"`
		Assignees   []string `json:"assignees,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "bad_request", "invalid JSON body", http.StatusBadRequest)
		return
	}

	task, err := h.svc.UpdateTask(r.Context(), taskID, req.Title, req.Description, req.Status, req.Assignees)
	if err != nil {
		switch err.(type) {
		case *storage.TaskNotFound:
			writeJSONError(w, "not_found", err.Error(), http.StatusNotFound)
		default:
			writeJSONError(w, "update_failed", err.Error(), http.StatusUnprocessableEntity)
		}
		return
	}

	reqLog(r).Info("api task updated", "task_id", task.ID, "status", task.Status)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

// deleteTask is the x402-protected API handler for
// DELETE /api/tasks/{id}. Deletes a task.
func (h *apiHandlers) deleteTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	reqLog(r).Info("api task deleted", "task_id", taskID)

	if err := h.svc.DeleteTask(r.Context(), taskID); err != nil {
		switch err.(type) {
		case *storage.TaskNotFound:
			writeJSONError(w, "not_found", err.Error(), http.StatusNotFound)
		default:
			writeJSONError(w, "delete_failed", err.Error(), http.StatusInternalServerError)
		}
		return
	}

	resp := map[string]interface{}{
		"ok":      true,
		"task_id": taskID,
		"status":  "deleted",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ── Helper ─────────────────────────────────────────────────────

// writeJSONError writes a uniform JSON error response.
func writeJSONError(w http.ResponseWriter, code, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   code,
		"message": message,
	})
}

// ── Server ─────────────────────────────────────────────────────────

// shutdownTimeout is the maximum time to wait for in-flight requests
// to complete after receiving a shutdown signal.
const shutdownTimeout = 10 * time.Second

// startTime records when the server started, used by the health
// endpoint to report uptime.
var startTime = time.Now()

// pinger is satisfied by any TaskStore implementation that can
// report database connectivity.
type pinger interface {
	Ping(ctx context.Context) error
}

// healthHandler returns the server health status as JSON.
// It checks:
//   - Server is alive (always true if this handler runs)
//   - Database connectivity via Ping
//   - Uptime since server start
//
// On failure it still returns 200 with a degraded field so that
// load balancers can route around an unhealthy database without
// dropping the connection entirely.
func healthHandler(p pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uptime := time.Since(startTime).Truncate(time.Second).String()

		resp := map[string]interface{}{
			"status":  "ok",
			"service": "taskflow",
			"uptime":  uptime,
		}

		// Check database connectivity.
		if err := p.Ping(r.Context()); err != nil {
			resp["status"] = "degraded"
			resp["database"] = map[string]interface{}{
				"status": "unreachable",
				"error":  err.Error(),
			}
		} else {
			resp["database"] = map[string]interface{}{
				"status": "connected",
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// newRouter builds the http.Handler (mux) with all routes and
// middleware wired. Extracted so tests and main() can share it.
// The pinger is used by the /healthz endpoint.
func newRouter(api *apiHandlers, p pinger) http.Handler {
	mux := http.NewServeMux()

	// ── x402 Payment Middleware ──────────────────────────────
	// Protects API endpoints so external tools / AI agents pay
	// per-call via the x402 protocol (https://x402.org).
	x402Cfg := middleware.X402Config{
		Price:    "$0.001",
		Currency: "USDC",
		Network:  "eip155:84532",
		PayTo:    "0xTaskFlowPayToAddress",
	}
	x402Mw := middleware.New(x402Cfg)

	// ── Health check (no auth, no payment) ───────────────────
	// GET /healthz and /health both report server status.
	// These are intentionally outside any middleware so they
	// work for load balancers, Kubernetes probes, etc.
	mux.HandleFunc("GET /healthz", healthHandler(p))
	mux.HandleFunc("GET /health", healthHandler(p))

	// ── Static assets ────────────────────────────────────────
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// ── Auth routes (no auth guard needed) ───────────────────
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/register", registerHandler)
	mux.HandleFunc("/logout", logoutHandler)

	// ── Protected HTML routes (session auth) ─────────────────
	mux.HandleFunc("/dashboard", requireAuth(dashboardHandler))
	mux.HandleFunc("/tasks", requireAuth(tasksHandler))
	mux.HandleFunc("/tasks/", requireAuth(tasksHandler))
	mux.HandleFunc("/upload", requireAuth(uploadHandler))
	mux.HandleFunc("/profile", requireAuth(profileHandler))
	mux.HandleFunc("/profile/", requireAuth(profileHandler))

	// ── x402-Protected API Routes (payment-gated) ────────────
	// External tools and AI agents pay per-call.
	mux.Handle("POST /api/tasks", x402Mw.Handler(http.HandlerFunc(api.createTask)))
	mux.Handle("GET /api/projects/{id}/tasks", x402Mw.Handler(http.HandlerFunc(api.getProjectTasks)))
	mux.Handle("PUT /api/tasks/{id}", x402Mw.Handler(http.HandlerFunc(api.updateTask)))
	mux.Handle("DELETE /api/tasks/{id}", x402Mw.Handler(http.HandlerFunc(api.deleteTask)))

	// ── Root redirect ─────────────────────────────────────────
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}
		http.NotFound(w, r)
	})

	// Wrap with request ID first, then logging — so the request
	// ID is available to the logging middleware and all handlers.
	return logging(requestID(mux))
}

// parseLogLevel reads the LOG_LEVEL environment variable and
// returns the corresponding slog.Level. Defaults to slog.LevelInfo
// if the variable is unset or unrecognized.
func parseLogLevel() slog.Level {
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ── Main ────────────────────────────────────────────────────────
func main() {
	// ── Structured JSON logging ───────────────────────────────
	// All server logs are emitted as newline-delimited JSON for
	// easy ingestion by log aggregators (Datadog, Loki, etc.).
	//
	// Set LOG_LEVEL to one of: debug, info, warn, error (default: info).
	logLevel := parseLogLevel()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: logLevel == slog.LevelDebug,
	})))

	// ── Dependencies ───────────────────────────────────────────
	// Use PostgreSQL when DATABASE_URL is set, otherwise fall
	// back to the in-memory store (great for development).
	var (
		store   storage.TaskStore
		closers []func() // things to clean up on shutdown
	)

	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		ctx := context.Background()
		pgStore, err := storage.NewPostgresTaskStore(ctx, dbURL)
		if err != nil {
			slog.Error("failed to connect to PostgreSQL", "error", err)
			os.Exit(1)
		}
		closers = append(closers, pgStore.Close)
		store = pgStore
		slog.Info("storage backend", "backend", "postgresql")
	} else {
		store = storage.NewInMemoryTaskStore()
		slog.Info("storage backend", "backend", "in-memory", "hint", "set DATABASE_URL for PostgreSQL")
	}

	svc := service.NewTaskService(store)
	api := newAPIHandlers(svc)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// ── HTTP Server ───────────────────────────────────────────
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: newRouter(api, store),
	}

	// ── Signal handling (graceful shutdown) ───────────────────
	// Listen for OS interrupt / terminate signals in a separate
	// goroutine. When one arrives, drain in-flight requests and
	// shut down cleanly.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Start the server in a goroutine so we can listen for signals.
	// If ListenAndServe fails for a reason other than a clean shutdown
	// (e.g. port already in use), we log.Fatalf immediately — there's
	// nothing to gracefully shut down in that case.
	go func() {
		slog.Info("server starting", "addr", ":"+port)
		slog.Info("x402 endpoints", "POST", "/api/tasks", "GET", "/api/projects/{id}/tasks", "PUT", "/api/tasks/{id}", "DELETE", "/api/tasks/{id}")
		slog.Info("graceful shutdown enabled — send SIGINT/SIGTERM to stop")

		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Block until we receive a signal.
	sig := <-stop
	slog.Warn("shutting down", "signal", sig.String())

	// Create a context with a timeout so the server can't wait
	// indefinitely for in-flight requests to finish.
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Shutdown gracefully: stop accepting new connections and
	// wait for active ones to finish (up to shutdownTimeout).
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown incomplete", "error", err)
		if err := srv.Close(); err != nil {
			slog.Error("force close error", "error", err)
		}
	}

	// Close any resources that need cleanup (e.g. Postgres pool).
	for _, closeFn := range closers {
		closeFn()
	}

	slog.Info("server stopped")
}
