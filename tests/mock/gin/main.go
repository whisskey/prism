package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
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

func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{Success: true, Data: data})
}

func JWTAuth(c *gin.Context) {
	if c.GetHeader("Authorization") == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Next()
}

func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("X-Role") != role {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}

func ValidateJSON[T any]() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body T
		if err := json.NewDecoder(c.Request.Body).Decode(&body); err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		c.Next()
	}
}

func paginationQuery(c *gin.Context) (int, int) {
	var q PaginationQuery
	q.Page, _ = strconv.Atoi(c.Query("page"))
	q.PerPage, _ = strconv.Atoi(c.Query("per_page"))
	return q.Page, q.PerPage
}

// ListUsers returns users.
// @openapi:summary List users
func ListUsers(c *gin.Context) {
	_, _ = paginationQuery(c)
	users := []User{{ID: "1", Name: "Ada"}}
	Success(c, users)
}

func GetUser(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	Success(c, User{ID: strconv.Itoa(id), Name: "Ada"})
}

// CreateUser creates a user.
// @openapi:summary Create user
func CreateUser(c *gin.Context) {
	var req CreateUserRequest
	_ = c.ShouldBindJSON(&req)
	Success(c, User{ID: "new", Name: req.Name})
}

func GetMe(c *gin.Context) {
	Success(c, User{ID: "me", Name: "Ada"})
}

func main() {
	r := gin.New()
	r.Use(JWTAuth)
	api := r.Group("/api/v1/users")
	api.GET("/me", GetMe)
	api.GET("/", RequireRole("admin"), ListUsers)
	api.POST("/", ValidateJSON[CreateUserRequest](), CreateUser)
	api.GET("/:id", GetUser)
	_ = r.Run(":8080")
}
