package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

const (
	refreshCookieName = "refreshToken"
)

type authService interface {
	Registration(context.Context, string, string, string) (RegistrationResponse, error)
	Login(context.Context, string, string) (AuthResult, error)
	Logout(context.Context, string) error
	Activate(context.Context, string) error
	ResendActivation(context.Context, string) error
	Refresh(context.Context, string) (AuthResult, error)
}

type AuthHandler struct {
	userService  authService
	log          *slog.Logger
	cookieSecure bool
}

func NewAuthHandler(
	log *slog.Logger,
	userService authService,
	cookieSecure bool,
) *AuthHandler {
	return &AuthHandler{
		log:          log.With("component", "handler", "entity", "auth"),
		userService:  userService,
		cookieSecure: cookieSecure,
	}
}

func (h *AuthHandler) Registration(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req RegistrationRequest
	if !bindAuthRequest(c, &req) {
		return
	}

	resp, err := h.userService.Registration(
		c.Request.Context(), req.Name, req.Email, req.Password,
	)
	if writeValidationError(c, err) {
		return
	}
	if errors.Is(err, ErrUserAlreadyExist) {
		writeAuthError(c, http.StatusConflict, "user_already_exists", "Пользователь с таким email уже существует")
		return
	}
	if errors.Is(err, ErrActivationDelivery) {
		writeAuthError(c, http.StatusServiceUnavailable, "activation_delivery_failed",
			"Аккаунт создан, но письмо не отправлено. Попробуйте запросить новое письмо позже.")
		return
	}
	if err != nil {
		h.log.Error("failed to register user")
		writeAuthError(c, http.StatusInternalServerError, "internal_error", "Не удалось зарегистрироваться")
		return
	}

	c.JSON(http.StatusCreated, resp)
}

func (h *AuthHandler) Login(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req LoginRequest
	if !bindAuthRequest(c, &req) {
		return
	}

	result, err := h.userService.Login(c.Request.Context(), req.Email, req.Password)
	if writeValidationError(c, err) {
		return
	}
	if errors.Is(err, ErrInvalidCredentials) {
		writeAuthError(c, http.StatusUnauthorized, "invalid_credentials", "Неверный email или пароль")
		return
	}
	if errors.Is(err, ErrUserNotActivated) {
		writeAuthError(c, http.StatusForbidden, "email_not_verified", "Подтвердите email перед входом")
		return
	}
	if err != nil {
		h.log.Error("failed to login")
		writeAuthError(c, http.StatusInternalServerError, "internal_error", "Не удалось выполнить вход")
		return
	}

	h.writeSession(c, result)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	refreshToken, err := c.Cookie(refreshCookieName)
	if err != nil {
		h.clearRefreshCookie(c)
		c.JSON(http.StatusOK, gin.H{"message": "Выход выполнен"})
		return
	}

	if err := h.userService.Logout(c.Request.Context(), refreshToken); err != nil {
		h.log.Error("failed to logout")
		writeAuthError(c, http.StatusInternalServerError, "internal_error", "Не удалось завершить сессию")
		return
	}

	h.clearRefreshCookie(c)
	c.JSON(http.StatusOK, gin.H{"message": "Выход выполнен"})
}

func (h *AuthHandler) Activate(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	var req ActivationRequest
	if !bindAuthRequest(c, &req) {
		return
	}
	err := h.userService.Activate(c.Request.Context(), req.Token)
	if errors.Is(err, ErrInvalidActivation) {
		writeAuthError(c, http.StatusBadRequest, "invalid_activation", "Ссылка недействительна или срок её действия истёк. Запросите новое письмо.")
		return
	}
	if err != nil {
		// An activation token may occur in a dependency error as well as the URL.
		h.log.Error("failed to activate user")
		writeAuthError(c, http.StatusInternalServerError, "internal_error", "Не удалось подтвердить email")
		return
	}

	c.Status(http.StatusNoContent)
}

func bindAuthRequest(c *gin.Context, req any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	payload, err := io.ReadAll(c.Request.Body)
	if err == nil {
		err = json.Unmarshal(payload, req)
	}
	if err == nil {
		err = binding.Validator.ValidateStruct(req)
	}
	if err == nil {
		err = validateAuthFields(req)
	}
	if err != nil {
		if writeValidationError(c, err) {
			return false
		}
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			writeAuthError(c, http.StatusRequestEntityTooLarge, "body_too_large", "Слишком большой запрос")
		} else {
			writeAuthError(c, http.StatusBadRequest, "validation_failed", "Проверьте поля запроса")
		}
		return false
	}
	return true
}

func (h *AuthHandler) ResendActivation(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req ResendActivationRequest
	if !bindAuthRequest(c, &req) {
		return
	}
	err := h.userService.ResendActivation(c.Request.Context(), req.Email)
	if writeValidationError(c, err) {
		return
	}
	var cooldown *ActivationCooldownError
	if errors.As(err, &cooldown) {
		c.Header("Retry-After", strconv.Itoa(cooldown.RetryAfter))
		writeAuthError(c, http.StatusTooManyRequests, "activation_cooldown", "Письмо уже запрошено. Подождите немного перед повторной отправкой.")
		return
	}
	if errors.Is(err, ErrActivationDelivery) {
		writeAuthError(c, http.StatusServiceUnavailable, "activation_delivery_failed", "Не удалось отправить письмо. Повторите запрос позже.")
		return
	}
	if err != nil {
		h.log.Error("failed to resend activation email")
		writeAuthError(c, http.StatusInternalServerError, "internal_error", "Не удалось запросить письмо")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "Если аккаунт ожидает подтверждения, на этот адрес отправлено письмо."})
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
		h.log.Error("failed to refresh token")
		writeAuthError(c, http.StatusInternalServerError, "internal_error", "Не удалось обновить сессию")
		return
	}

	h.writeSession(c, result)
}

func writeAuthError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"code": code, "message": message})
}

func writeValidationError(c *gin.Context, err error) bool {
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		return false
	}
	c.JSON(http.StatusBadRequest, gin.H{
		"code":        "validation_failed",
		"message":     "Проверьте поля запроса",
		"fieldErrors": invalid.Fields,
	})
	return true
}

func (h *AuthHandler) writeSession(c *gin.Context, result AuthResult) {
	h.setRefreshCookie(c, result.Tokens.RefreshToken, result.Tokens.RefreshExpiresAt)
	c.JSON(http.StatusOK, AuthResponse{
		User:        result.User,
		AccessToken: result.Tokens.AccessToken,
	})
}

func (h *AuthHandler) setRefreshCookie(c *gin.Context, token string, expiresAt time.Time) {
	age := int(math.Ceil(time.Until(expiresAt).Seconds()))
	if age <= 0 {
		h.clearRefreshCookie(c)
		return
	}
	h.writeRefreshCookie(c, token, age, expiresAt)
}

func (h *AuthHandler) clearRefreshCookie(c *gin.Context) {
	h.writeRefreshCookie(c, "", -1, time.Unix(1, 0))
}

func (h *AuthHandler) writeRefreshCookie(c *gin.Context, token string, age int, expiresAt time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: refreshCookieName, Value: token, Path: "/", MaxAge: age,
		Expires: expiresAt, HttpOnly: true, Secure: h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}
