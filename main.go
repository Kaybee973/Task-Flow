package main

import (
	"context"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

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
		log.Println("template execute:", err)
	}
}

// ── Middleware ──────────────────────────────────────────────────
// Simple logging middleware — wraps any handler.
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
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
		password := r.FormValue("password")
		log.Printf("Login attempt: %s / %s", email, password)
		// TODO: query DB, bcrypt.CompareHashAndPassword, set cookie
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
		log.Printf("Register: %s / %s", name, email)
		if password != confirm {
			http.Error(w, "Passwords do not match", http.StatusBadRequest)
			return
		}
		_ = password // TODO: bcrypt.GenerateFromPassword, insert into DB
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
			log.Printf("Create task: %s", title)
			// TODO: INSERT into DB
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		} else {
			serveTemplate(w, "dashboard.html", nil)
		}

	// /tasks/{id}/delete
	case sub == "delete":
		log.Printf("Delete task %s", id)
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
				log.Printf("Uploaded: %s (%d bytes) for task %s", fh.Filename, fh.Size, id)
				// TODO: save file, record in DB
			}
			http.Redirect(w, r, "/tasks/"+id, http.StatusSeeOther)
		}

	// /tasks/{id}
	default:
		if r.Method == http.MethodPost {
			log.Printf("Update task %s: %s", id, r.FormValue("title"))
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
			log.Printf("Upload: %s (%d bytes)", fh.Filename, fh.Size)
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

	task, err := h.svc.CreateTask(req.Title, req.Description, req.ProjectID, req.Assignees)
	if err != nil {
		writeJSONError(w, "creation_failed", err.Error(), http.StatusUnprocessableEntity)
		return
	}

	log.Printf("API created task: id=%s title=%q project=%s", task.ID, task.Title, task.ProjectID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

// getProjectTasks is the x402-protected API handler for
// GET /api/projects/{id}/tasks. Returns all tasks for a project.
func (h *apiHandlers) getProjectTasks(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	log.Printf("API get project tasks: project=%s", projectID)

	tasks, err := h.svc.GetProjectTasks(projectID)
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

	task, err := h.svc.UpdateTask(taskID, req.Title, req.Description, req.Status, req.Assignees)
	if err != nil {
		switch err.(type) {
		case *storage.TaskNotFound:
			writeJSONError(w, "not_found", err.Error(), http.StatusNotFound)
		default:
			writeJSONError(w, "update_failed", err.Error(), http.StatusUnprocessableEntity)
		}
		return
	}

	log.Printf("API updated task: id=%s status=%q", task.ID, task.Status)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

// deleteTask is the x402-protected API handler for
// DELETE /api/tasks/{id}. Deletes a task.
func (h *apiHandlers) deleteTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	log.Printf("API delete task: id=%s", taskID)

	if err := h.svc.DeleteTask(taskID); err != nil {
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

// ── Main ────────────────────────────────────────────────────────
func main() {
	// ── Dependencies ───────────────────────────────────────────
	// Use PostgreSQL when DATABASE_URL is set, otherwise fall
	// back to the in-memory store (great for development).
	var store storage.TaskStore
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		ctx := context.Background()
		pgStore, err := storage.NewPostgresTaskStore(ctx, dbURL)
		if err != nil {
			log.Fatalf("failed to connect to PostgreSQL: %v", err)
		}
		defer pgStore.Close()
		store = pgStore
		log.Println("Using PostgreSQL storage")
	} else {
		store = storage.NewInMemoryTaskStore()
		log.Println("Using in-memory storage (set DATABASE_URL for PostgreSQL)")
	}

	svc := service.NewTaskService(store)
	api := newAPIHandlers(svc)

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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server running at http://localhost:%s", port)
	log.Printf("x402-protected API endpoints:")
	log.Printf("  POST   /api/tasks")
	log.Printf("  GET    /api/projects/{id}/tasks")
	log.Printf("  PUT    /api/tasks/{id}")
	log.Printf("  DELETE /api/tasks/{id}")
	log.Fatal(http.ListenAndServe(":"+port, logging(mux)))
}
