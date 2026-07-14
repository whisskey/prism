package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Response struct {
	Success bool `json:"success"`
	Data    any  `json:"data,omitempty"`
	Error   any  `json:"error,omitempty"`
}

type PaginationQuery struct {
	Page    int `json:"page" query:"page"`
	PerPage int `json:"per_page" query:"per_page"`
}

func Success(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Response{Success: true, Data: data})
}

func JWTAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Role") != role {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ValidateJSON[T any]() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body T
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func paginationQuery(r *http.Request) (int, int) {
	var q PaginationQuery
	q.Page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	q.PerPage, _ = strconv.Atoi(r.URL.Query().Get("per_page"))
	return q.Page, q.PerPage
}

// ListUsers returns users.
// @openapi:summary List users
func ListUsers(w http.ResponseWriter, r *http.Request) {
	_, _ = paginationQuery(r)
	users := []User{{ID: "1", Name: "Ada"}}
	Success(w, users)
}

func GetUser(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	Success(w, User{ID: strconv.Itoa(id), Name: "Ada"})
}

// CreateUser creates a user.
// @openapi:summary Create user
func CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	Success(w, User{ID: "new", Name: req.Name})
}

func GetMe(w http.ResponseWriter, r *http.Request) {
	Success(w, User{ID: "me", Name: "Ada"})
}

func main() {
	r := chi.NewRouter()
	r.Use(JWTAuth)
	r.Route("/api/v1/users", func(r chi.Router) {
		r.Get("/me", GetMe)
		r.With(RequireRole("admin")).Get("/", ListUsers)
		r.With(ValidateJSON[CreateUserRequest]()).Post("/", CreateUser)
		r.Get("/{id}", GetUser)
	})
	_ = http.ListenAndServe(":8080", r)
}
