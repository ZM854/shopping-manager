package auth

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	userService *UserService
	log *slog.Logger
}

func NewAuthHandler(log *slog.Logger, userService *UserService) *AuthHandler {
	return &AuthHandler{
		log: log.With("component", "handler", "entity", "auth"),
		userService: userService,
	}
}

func (AuthHandler *AuthHandler) Registration(c *gin.Context)  {
	c.JSON(200, gin.H{
		"working": "duncan",
	})
}

func (AuthHandler *AuthHandler) Login(c *gin.Context)  {
	c.JSON(200, gin.H{
		"working": "duncan",
	})
}

func (AuthHandler *AuthHandler) Logout(c *gin.Context)  {
	c.JSON(200, gin.H{
		"working": "duncan",
	})
}

func (AuthHandler *AuthHandler) Activate(c *gin.Context)  {
	c.JSON(200, gin.H{
		"working": "duncan",
	})
}

func (AuthHandler *AuthHandler) Refresh(c *gin.Context)  {
	c.JSON(200, gin.H{
		"working": "duncan",
	})
}

func (AuthHandler *AuthHandler) GetUsers(c *gin.Context)  {
	c.JSON(200, gin.H{
		"working": "duncan",
	})
}