package router

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/ZM854/shopping-manager/backend/internal/media"
	"github.com/ZM854/shopping-manager/backend/internal/middleware"
	"github.com/ZM854/shopping-manager/backend/internal/recipe"
	"github.com/ZM854/shopping-manager/backend/internal/shopping"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func New(
	log *slog.Logger,
	shoppingHandler *shopping.Handler,
	recipeHandler *recipe.Handler,
	mediaHandler *media.Handler,
	authHandler *auth.AuthHandler,
	authMiddleware *middleware.AuthMiddleware,
	cfg config.Config,
) *gin.Engine {
	router := gin.New()
	if err := router.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		panic("invalid trusted proxy configuration")
	}
	router.Use(gin.Recovery())
	router.Use(middleware.RequestLogger(log))
	// Set the policy before Origin and authentication can reject a sharing request.
	router.Use(func(c *gin.Context) {
		switch c.FullPath() {
		case "/api/shopping-lists/:listId/shares", "/api/shopping-list-shares/import":
			c.Header("Cache-Control", "no-store")
		}
		path := c.Request.URL.Path
		if path == "/api/recipe-categories" || path == "/api/recipes" || strings.HasPrefix(path, "/api/recipes/") || path == "/api/recipe-media" || strings.HasPrefix(path, "/api/recipe-media/") {
			c.Header("Cache-Control", "no-store")
		}
		c.Next()
		drainRejectedMedia(c)
	})
	router.Use(middleware.CheckOrigin(cfg.AllowedOrigins))

	router.Use(cors.New(cors.Config{
		AllowOrigins: cfg.AllowedOrigins,
		AllowMethods: []string{
			"GET",
			"POST",
			"PUT",
			"PATCH",
			"DELETE",
			"OPTIONS",
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
			"Idempotency-Key",
		},
		AllowCredentials: true,
		ExposeHeaders:    []string{"Retry-After"},
		MaxAge:           12 * time.Hour,
	}))

	api := router.Group("/api")
	api.Use(middleware.NewRateLimiter(map[string]middleware.RateRule{
		"/api/login":             {Limit: cfg.AuthLoginLimit, Window: time.Minute},
		"/api/registration":      {Limit: cfg.AuthRegistrationLimit, Window: time.Hour},
		"/api/activation/resend": {Limit: cfg.AuthResendLimit, Window: 10 * time.Minute},
	}).Handle())
	api.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	api.POST("/registration", authHandler.Registration)
	api.POST("/login", authHandler.Login)
	api.POST("/logout", authHandler.Logout)
	api.POST("/activation", authHandler.Activate)
	api.POST("/activation/resend", authHandler.ResendActivation)
	api.POST("/refresh", authHandler.Refresh)

	privateAPI := api.Group("")
	privateAPI.Use(authMiddleware.HandleAuth())
	shoppingHandler.Register(privateAPI)
	recipeHandler.Register(privateAPI)
	if mediaHandler != nil {
		mediaHandler.Register(privateAPI)
	}

	return router
}


func drainRejectedMedia(c *gin.Context) {
	path := c.Request.URL.Path
	if c.Writer.Status() < 400 || c.Request.Method != http.MethodPut ||
		!strings.HasPrefix(path, "/api/recipe-media/") || !strings.HasSuffix(path, "/content") {
		return
	}
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetReadDeadline(time.Now().Add(5 * time.Second))
	const maxDiscard int64 = (50 << 20) + 1
	n, err := io.Copy(io.Discard, io.LimitReader(c.Request.Body, maxDiscard))
	if err == nil && n < maxDiscard {
		_ = controller.SetReadDeadline(time.Time{})
	}
}
