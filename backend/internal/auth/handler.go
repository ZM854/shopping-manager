package auth

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	log *slog.Logger
}

func NewAuthHandler(log *slog.Logger) *AuthHandler {
	return &AuthHandler{
		log: log.With("component", "handler", "entity", "auth"),
	}
}

func (AuthHandler *AuthHandler) Registration(c *gin.Context)  {
	
}

func (AuthHandler *AuthHandler) Login(c *gin.Context)  {
	
}

func (AuthHandler *AuthHandler) Logout(c *gin.Context)  {
	
}

func (AuthHandler *AuthHandler) Activate(c *gin.Context)  {
	
}

func (AuthHandler *AuthHandler) Refresh(c *gin.Context)  {
	
}

func (AuthHandler *AuthHandler) GetUsers(c *gin.Context)  {
	
}