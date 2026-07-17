package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
		// cookie, err := r.Cookie("session_token")
		// if err != nil { http.Redirect(w, r, "/login", http.StatusSeeOther); return }
		next(w, r)
	}
}

// ── Handlers ────────────────────────────────────────────────────

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

// GET  /tasks            — list all tasks
// POST /tasks            — create task
// GET  /tasks/{id}       — task detail
// POST /tasks/{id}       — update task
// POST /tasks/{id}/delete
// POST /tasks/{id}/attachments — file upload
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
			// f, _ := fh.Open(); defer f.Close()
			// dst, _ := os.Create("./uploads/" + fh.Filename)
			// io.Copy(dst, f)
		}
		http.Redirect(w, r, "/upload", http.StatusSeeOther)
	}
}

// GET/POST /profile and sub-routes
func profileHandler(w http.ResponseWriter, r *http.Request) {
	serveTemplate(w, "profile.html", nil)
}

// ── Main ────────────────────────────────────────────────────────
func main() {
	mux := http.NewServeMux()

	// Serve static assets: CSS, JS, images
	// Files in ./static/ are served at /static/
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// Auth routes (no auth guard needed)
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/register", registerHandler)
	mux.HandleFunc("/logout", logoutHandler)

	// Protected routes — wrap with requireAuth in real app
	mux.HandleFunc("/dashboard", requireAuth(dashboardHandler))
	mux.HandleFunc("/tasks", requireAuth(tasksHandler))
	mux.HandleFunc("/tasks/", requireAuth(tasksHandler)) // catches /tasks/{id} and sub-paths
	mux.HandleFunc("/upload", requireAuth(uploadHandler))
	mux.HandleFunc("/profile", requireAuth(profileHandler))
	mux.HandleFunc("/profile/", requireAuth(profileHandler))

	// Root redirect
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
	log.Fatal(http.ListenAndServe(":"+port, logging(mux)))
}

type NoDirFs struct {
	fs http.FileSystem
}

func (ndf *NoDirFs) Open(path string) (http.File, error) {
	file, err := ndf.fs.Open(path)
	if err != nil {
		return nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if stat.IsDir() {
		index := filepath.Join(path, "index.html")
		if _, err := ndf.fs.Open(index); err != nil {
			file.Close()
			return nil, err
		}
	}
	return file, nil
}
