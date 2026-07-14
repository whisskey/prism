package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
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

func Success(c echo.Context, data any) error {
	return c.JSON(http.StatusOK, Response{Success: true, Data: data})
}

func JWTAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if c.Request().Header.Get("Authorization") == "" {
			return c.NoContent(http.StatusUnauthorized)
		}
		return next(c)
	}
}

func RequireRole(role string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().Header.Get("X-Role") != role {
				return c.NoContent(http.StatusForbidden)
			}
			return next(c)
		}
	}
}

func ValidateJSON[T any]() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			var body T
			if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
				return c.NoContent(http.StatusBadRequest)
			}
			return next(c)
		}
	}
}

func paginationQuery(c echo.Context) (int, int) {
	var q PaginationQuery
	q.Page, _ = strconv.Atoi(c.QueryParam("page"))
	q.PerPage, _ = strconv.Atoi(c.QueryParam("per_page"))
	return q.Page, q.PerPage
}

// ListUsers returns users.
// @openapi:summary List users
func ListUsers(c echo.Context) error {
	_, _ = paginationQuery(c)
	users := []User{{ID: "1", Name: "Ada"}}
	return Success(c, users)
}

func GetUser(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	return Success(c, User{ID: strconv.Itoa(id), Name: "Ada"})
}

// CreateUser creates a user.
// @openapi:summary Create user
func CreateUser(c echo.Context) error {
	var req CreateUserRequest
	_ = c.Bind(&req)
	return Success(c, User{ID: "new", Name: req.Name})
}

func GetMe(c echo.Context) error {
	return Success(c, User{ID: "me", Name: "Ada"})
}

func main() {
	e := echo.New()
	e.Use(JWTAuth)
	api := e.Group("/api/v1/users")
	api.GET("/me", GetMe)
	api.GET("/", ListUsers, RequireRole("admin"))
	api.POST("/", CreateUser, ValidateJSON[CreateUserRequest]())
	api.GET("/:id", GetUser)
	_ = e.Start(":8080")
}
