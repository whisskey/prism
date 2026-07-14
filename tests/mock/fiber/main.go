package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v2"
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

func Success(c *fiber.Ctx, data any) error {
	return c.Status(http.StatusOK).JSON(Response{Success: true, Data: data})
}

func JWTAuth(c *fiber.Ctx) error {
	if c.Get("Authorization") == "" {
		return c.SendStatus(http.StatusUnauthorized)
	}
	return c.Next()
}

func RequireRole(role string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Get("X-Role") != role {
			return c.SendStatus(http.StatusForbidden)
		}
		return c.Next()
	}
}

func ValidateJSON[T any]() fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body T
		if err := json.Unmarshal(c.Body(), &body); err != nil {
			return c.SendStatus(http.StatusBadRequest)
		}
		return c.Next()
	}
}

func paginationQuery(c *fiber.Ctx) (int, int) {
	var q PaginationQuery
	q.Page, _ = strconv.Atoi(c.Query("page"))
	q.PerPage, _ = strconv.Atoi(c.Query("per_page"))
	return q.Page, q.PerPage
}

// ListUsers returns users.
// @openapi:summary List users
func ListUsers(c *fiber.Ctx) error {
	_ = c.Query("page")
	_ = c.Query("per_page")
	_, _ = paginationQuery(c)
	users := []User{{ID: "1", Name: "Ada"}}
	return Success(c, users)
}

func GetUser(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	return Success(c, User{ID: strconv.Itoa(id), Name: "Ada"})
}

// CreateUser creates a user.
// @openapi:summary Create user
func CreateUser(c *fiber.Ctx) error {
	var req CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.SendStatus(http.StatusBadRequest)
	}
	return Success(c, User{ID: "new", Name: req.Name})
}

func GetMe(c *fiber.Ctx) error {
	return Success(c, User{ID: "me", Name: "Ada"})
}

func main() {
	app := fiber.New()
	app.Use(JWTAuth)
	api := app.Group("/api/v1/users")
	api.Get("/me", GetMe)
	api.Get("/", RequireRole("admin"), ListUsers)
	api.Post("/", ValidateJSON[CreateUserRequest](), CreateUser)
	api.Get("/:id", GetUser)
	_ = app.Listen(":8080")
}
