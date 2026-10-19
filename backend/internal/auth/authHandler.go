package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	refreshCookieName = "refreshToken"
	refreshCookieAge  = 30 * 24 * 60 * 60
)

type authService interface {
	Registration(context.Context, string, string, string) (RegistrationResponse, error)
	Login(context.Context, string, string) (AuthResult, error)
	Logout(context.Context, string) error
	Activate(context.Context, string) error
	Refresh(context.Context, string) (AuthResult, error)
}

type AuthHandler struct {
	userService       authService
	log               *slog.Logger
	clientRedirectURL string
}

func NewAuthHandler(
	log *slog.Logger,
	userService authService,
	clientRedirectURL string,
) *AuthHandler {
	return &AuthHandler{
		log:               log.With("component", "handler", "entity", "auth"),
		userService:       userService,
		clientRedirectURL: clientRedirectURL,
	}
}

func (h *AuthHandler) Registration(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req RegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAuthError(c, http.StatusBadRequest, "validation_failed", "Проверьте поля запроса")
		return
	}

	resp, err := h.userService.Registration(
		c.Request.Context(), req.Name, req.Email, req.Password,
	)
	if errors.Is(err, ErrUserAlreadyExist) {
		writeAuthError(c, http.StatusConflict, "user_already_exists", "Пользователь с таким email уже существует")
		return
	}
	if err != nil {
		h.log.Error("failed to register user", "error", err)
		writeAuthError(c, http.StatusInternalServerError, "internal_error", "Не удалось зарегистрироваться")
		return
	}

	c.JSON(http.StatusCreated, resp)
}

func (h *AuthHandler) Login(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAuthError(c, http.StatusBadRequest, "validation_failed", "Проверьте поля запроса")
		return
	}

	result, err := h.userService.Login(c.Request.Context(), req.Email, req.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		writeAuthError(c, http.StatusUnauthorized, "invalid_credentials", "Неверный email или пароль")
		return
	}
	if errors.Is(err, ErrUserNotActivated) {
		writeAuthError(c, http.StatusForbidden, "email_not_verified", "Подтвердите email перед входом")
		return
	}
	if err != nil {
		h.log.Error("failed to login", "error", err)
		writeAuthError(c, http.StatusInternalServerError, "internal_error", "Не удалось выполнить вход")
		return
	}

	h.writeSession(c, result)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	refreshToken, err := c.Cookie(refreshCookieName)
	if err != nil {
		writeAuthError(c, http.StatusUnauthorized, "unauthorized", "Требуется авторизация")
		return
	}

	if err := h.userService.Logout(c.Request.Context(), refreshToken); err != nil {
		h.log.Error("failed to logout", "error", err)
		writeAuthError(c, http.StatusInternalServerError, "internal_error", "Не удалось завершить сессию")
		return
	}

	h.clearRefreshCookie(c)
	c.JSON(http.StatusOK, gin.H{"message": "Выход выполнен"})
}

func (h *AuthHandler) Activate(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	err := h.userService.Activate(c.Request.Context(), c.Param("link"))
	if errors.Is(err, ErrInvalidActivation) {
		writeAuthError(c, http.StatusBadRequest, "invalid_activation", "Недействительная ссылка активации")
		return
	}
	if err != nil {
		// An activation token may occur in a dependency error as well as the URL.
		h.log.Error("failed to activate user")
		writeAuthError(c, http.StatusInternalServerError, "internal_error", "Не удалось подтвердить email")
		return
	}

	c.Redirect(http.StatusFound, h.clientRedirectURL)
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	refreshToken, err := c.Cookie(refreshCookieName)
	if err != nil {
		writeAuthError(c, http.StatusUnauthorized, "unauthorized", "Требуется авторизация")
		return
	}

	result, err := h.userService.Refresh(c.Request.Context(), refreshToken)
	if errors.Is(err, ErrUserNotActivated) {
		writeAuthError(c, http.StatusForbidden, "email_not_verified", "Подтвердите email перед входом")
		return
	}
	if errors.Is(err, ErrInvalidToken) || errors.Is(err, ErrRefreshTokenNotFound) ||
		errors.Is(err, ErrUserNotFound) {
		writeAuthError(c, http.StatusUnauthorized, "unauthorized", "Сессия недействительна, войдите снова")
		return
	}
	if err != nil {
		h.log.Error("failed to refresh token", "error", err)
		writeAuthError(c, http.StatusInternalServerError, "internal_error", "Не удалось обновить сессию")
		return
	}

	h.writeSession(c, result)
}

func writeAuthError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"code": code, "message": message})
}

func (h *AuthHandler) writeSession(c *gin.Context, result AuthResult) {
	h.setRefreshCookie(c, result.Tokens.RefreshToken)
	c.JSON(http.StatusOK, AuthResponse{
		User:        result.User,
		AccessToken: result.Tokens.AccessToken,
	})
}

func (h *AuthHandler) setRefreshCookie(c *gin.Context, token string) {
	c.SetCookie(refreshCookieName, token, refreshCookieAge, "/", "", false, true)
}

func (h *AuthHandler) clearRefreshCookie(c *gin.Context) {
	c.SetCookie(refreshCookieName, "", -1, "/", "", false, true)
}
