package auth

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	refreshCookieName = "refreshToken"
	refreshCookieAge = 30 * 24 * 60 * 60
	clientRedirectURL = "http://localhost:5173"
)

type AuthHandler struct {
	userService *UserService
	log *slog.Logger
}

func NewAuthHandler(
	log *slog.Logger, 
	userService *UserService,
) *AuthHandler {
	return &AuthHandler{
		log: log.With("component", "handler", "entity", "auth"),
		userService: userService,
	}
}

func (h *AuthHandler) Registration(c *gin.Context)  {
	var req RegistrationRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	resp, err := h.userService.Registration(
		c.Request.Context(),
		req.Email,
		req.Password,
	)

	if errors.Is(err, ErrUserAlreadyExist) {
		c.JSON(http.StatusConflict, gin.H{
			"error": "user already exist",
		})
		return
	}

	if err != nil {
		h.log.Error(
			"failed to register user",
			"email", req.Email,
			"error", err,
		)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to register user",
		})
		return
	}

	h.setRefreshCookie(c, resp.RefreshToken)

	c.JSON(http.StatusCreated, resp)
}

func (h *AuthHandler) Login(c *gin.Context)  {
	var req LoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	resp, err := h.userService.Login(
		c.Request.Context(),
		req.Email,
		req.Password,
	)

	if errors.Is(err, ErrInvalidCredentials) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid email or password",
		})
		return
	}

	if errors.Is(err, ErrUserNotActivated) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "email is not activated",
		})
		return
	}

	if err != nil {
		h.log.Error(
			"failed to login", 
			"email", req.Email,
			"error", err,
		)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to login",
		})
		return
	}

	h.setRefreshCookie(c, resp.RefreshToken)

	c.JSON(http.StatusOK, resp)
}

func (h *AuthHandler) Logout(c *gin.Context)  {
	refreshToken, err := c.Cookie(refreshCookieName)

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "refresh token is missing",
		})
		return
	}

	err = h.userService.Logout(c.Request.Context(), refreshToken)

	if err != nil {
		h.log.Error(
			"failed to logout", 
			"error", err,
		)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to logout",
		})	
		return
	}

	h.clearRefreshCookie(c)
	
	c.JSON(http.StatusOK, gin.H{
		"message": "logged out",
	})
}

func (h *AuthHandler) Activate(c *gin.Context)  {
	activationLink := c.Param("link")


	err := h.userService.Activate(c.Request.Context(), activationLink)

	if errors.Is(err, ErrInvalidActivation) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid activation link",
		})
		return
	}

	if err != nil {
		h.log.Error(
			"failed to activate user", 
			"activation_link", activationLink,
			"error", err,
		)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to activate email",
		})
		return 
	}

	c.Redirect(http.StatusOK, clientRedirectURL)
}

func (h *AuthHandler) Refresh(c *gin.Context)  {
	refreshToken, err := c.Cookie(refreshCookieName)

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "refresh token is missing",
		})
		return
	}

	resp, err := h.userService.Refresh(c.Request.Context(), refreshToken)

	if err != nil {
		h.log.Error(
			"failed to refresh token",
			"error", err,
		)

		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid refresh token",
		})
		return
	}

	h.setRefreshCookie(c, resp.RefreshToken)

	c.JSON(http.StatusOK, resp)
}

func (h *AuthHandler) GetUsers(c *gin.Context)  {
	users, err := h.userService.GetAllUsers(c.Request.Context())

	if err != nil {
		h.log.Error(
			"failed to get users", 
			"error", err,
		)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get users",
		})
		return
	}

	c.JSON(http.StatusOK, users)
}

func (h *AuthHandler) setRefreshCookie(
	c *gin.Context,
	token string,
) {
	c.SetCookie(
		refreshCookieName,
		token,
		refreshCookieAge,
		"/",
		"",
		false,
		true,
	)
}

func (h *AuthHandler) clearRefreshCookie(c *gin.Context) {
	c.SetCookie(
		refreshCookieName,
		"",
		-1,
		"/",
		"",
		false,
		true,
	)
}