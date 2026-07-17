# Taskr — Task Manager

A complete static frontend of Task Manager

## Project structure

```
taskmanager/
├── main.go                  ← Starter Go server (all routes scaffolded)
├── login.html               ← GET /login, POST /login
├── register.html            ← GET /register, POST /register
├── dashboard.html           ← GET /dashboard (task list + kanban)
├── task-detail.html         ← GET /tasks/{id}, POST /tasks/{id}
├── upload.html              ← GET /upload, POST /upload
├── profile.html             ← GET /profile, POST /profile (+ sub-routes)
└── static/
    ├── css/style.css
    ├── js/app.js
    └── images/
        ├── logo.svg
        ├── empty-tasks.svg
        └── auth-illustration.svg
```

## Quick start

```bash
cd taskmanager
go run main.go
# Open http://localhost:8080
```

## Routes map

| Method | Path                          | Page / handler         | Practice topic          |
|--------|-------------------------------|------------------------|-------------------------|
| GET    | /login                        | login.html             | Template rendering      |
| POST   | /login                        | loginHandler           | Form parsing, sessions  |
| GET    | /register                     | register.html          | Template rendering      |
| POST   | /register                     | registerHandler        | bcrypt, DB insert       |
| POST   | /logout                       | logoutHandler          | Cookie deletion         |
| GET    | /dashboard                    | dashboard.html         | Auth middleware         |
| GET    | /tasks                        | dashboard.html         | DB query, template data |
| POST   | /tasks                        | tasksHandler           | Form POST, DB insert    |
| GET    | /tasks/{id}                   | task-detail.html       | Dynamic routing         |
| POST   | /tasks/{id}                   | tasksHandler           | DB update               |
| POST   | /tasks/{id}/delete            | tasksHandler           | DB delete               |
| POST   | /tasks/{id}/attachments       | tasksHandler           | Multipart file upload   |
| GET    | /upload                       | upload.html            | Template rendering      |
| POST   | /upload                       | uploadHandler          | Multipart, io.Copy      |
| GET    | /profile                      | profile.html           | Session user data       |
| POST   | /profile                      | profileHandler         | Form POST, DB update    |
| POST   | /profile/password             | profileHandler         | bcrypt compare + hash   |
| POST   | /profile/avatar               | profileHandler         | Image upload            |
| GET    | /static/*                     | http.FileServer        | Static file serving     |

## Go concepts exercised

- **Form handling** — `r.FormValue()`, `r.ParseForm()`
- **Auth** — sessions via cookies, bcrypt password hashing
- **File uploads** — `r.ParseMultipartForm()`, `r.FormFile()`, `io.Copy()`
- **Dynamic routing** — parsing `/tasks/{id}` from `r.URL.Path`
- **Static files** — `http.FileServer` + `http.StripPrefix`
- **Middleware** — logging, auth guard wrapping `http.HandlerFunc`
- **Templates** — `html/template.ParseFiles()`, `.Execute()`

## How to use the HTML as Go templates

Each HTML file is ready to be used with Go's `html/template`.
Replace the hardcoded values with template actions:

```html
<!-- In dashboard.html -->
<h1 class="page-title">Good morning, {{.User.Name}} 👋</h1>

<!-- Loop over tasks -->
{{range .Tasks}}
<div class="task-item" data-status="{{.Status}}">
  <div class="task-title">{{.Title}}</div>
</div>
{{end}}
```

Pass a struct from your handler:

```go
type PageData struct {
    User  User
    Tasks []Task
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
    data := PageData{
        User:  User{Name: "Jane"},
        Tasks: db.GetTasksForUser(userID),
    }
    serveTemplate(w, "dashboard.html", data)
}
```
