package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/ZM854/shopping-manager/backend/internal/database"
	"github.com/ZM854/shopping-manager/backend/internal/logger"
	"github.com/ZM854/shopping-manager/backend/internal/media"
	"github.com/ZM854/shopping-manager/backend/internal/middleware"
	"github.com/ZM854/shopping-manager/backend/internal/recipe"
	"github.com/ZM854/shopping-manager/backend/internal/router"
	"github.com/ZM854/shopping-manager/backend/internal/shopping"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid application configuration", "error", err)
		os.Exit(1)
	}

	appEnv := cfg.AppENV

	log := logger.New(appEnv)

	log.Info("starting application", "env", appEnv)
	mediaCtx, cancelMedia := context.WithTimeout(context.Background(), 45*time.Second)
	storage := media.NewStorage(cfg.Media)
	if err := storage.Check(mediaCtx); err != nil {
		cancelMedia()
		log.Error("media storage unavailable")
		os.Exit(1)
	}
	if err := storage.ConfigureBrowserAccess(mediaCtx, cfg.AllowedOrigins); err != nil {
		cancelMedia()
		log.Error("media storage CORS unavailable")
		os.Exit(1)
	}
	if err := media.CheckTools(mediaCtx, cfg.Media); err != nil {
		cancelMedia()
		log.Error("media tools unavailable")
		os.Exit(1)
	}
	cancelMedia()
	log.Info("media dependencies ready")

	db, err := database.NewPostgres(cfg, log)
	if err != nil {
		log.Error("failed to startup application", "error", err)
		os.Exit(1)
	}
	defer func() {
		log.Info("closing postgres connection pool")
		db.Close()
	}()

	log.Info("initializing dependencies")

	shoppingRepo := shopping.NewRepository(db)
	shoppingService := shopping.NewService(shoppingRepo)
	shoppingHandler := shopping.NewHandler(shoppingService, log)
	recipeRepo := recipe.NewRepository(db)
	recipeService := recipe.NewService(recipeRepo)
	recipeHandler := recipe.NewHandler(recipeService, log)
	mediaRepo := media.NewRepository(db)
	mediaService := media.NewService(mediaRepo)
	processor := media.MediaProcessor{
		Image: media.ImageProcessor{FFmpegPath: cfg.Media.FFmpegPath},
		Video: media.VideoProcessor{FFmpegPath: cfg.Media.FFmpegPath, FFprobePath: cfg.Media.FFprobePath},
	}
	uploader, err := media.NewUploader(mediaService, storage, processor, filepath.Join(os.TempDir(), "shopping-manager-media"))
	if err != nil {
		log.Error("media temporary directory unavailable")
		os.Exit(1)
	}
	recoverCtx, cancelRecover := context.WithTimeout(context.Background(), 30*time.Second)
	err = mediaRepo.RecoverInterrupted(recoverCtx)
	cancelRecover()
	if err != nil {
		log.Error("media recovery failed")
		os.Exit(1)
	}
	mediaHandler := media.NewHandler(mediaService, uploader, media.NewAccessService(mediaRepo, storage), log)

	tokenRepo := auth.NewTokenRepository(db, log)
	tokenService := auth.NewTokenService(
		log,
		tokenRepo,
		cfg.JWTAccessSecret,
		cfg.JWTRefreshSecret,
		cfg.JWTAccessTTL,
		cfg.JWTRefreshTTL,
	)

	smtpPort, err := strconv.Atoi(cfg.SMTPPort)

	if err != nil {
		log.Error("invalid smtp port", "port", cfg.SMTPPort)
		os.Exit(1)
	}

	mailService, err := auth.NewMailService(
		log,
		cfg.SMTPHost,
		smtpPort,
		cfg.SMTPUser,
		cfg.SMTPPassword,
		cfg.SMTPFrom,
		cfg.SMTPAuthEnabled,
		cfg.SMTPTLSRequired,
	)

	if err != nil {
		log.Error("failed to initialize mail service", "error", err)
		os.Exit(1)
	}

	userRepo := auth.NewUserRepository(db)
	userService := auth.NewUserService(
		log,
		userRepo,
		tokenService,
		mailService,
		cfg.FrontendURL+"/activation",
		auth.ActivationOptions{
			TTL:            cfg.ActivationTTL,
			ResendInterval: cfg.ActivationResendInterval,
			MailTimeout:    cfg.ActivationMailTimeout,
		},
	)
	authHandler := auth.NewAuthHandler(log, userService, cfg.CookieSecure)

	authMiddleware := middleware.NewAuthMiddleware(tokenService)

	router := router.New(
		log,
		shoppingHandler,
		recipeHandler,
		mediaHandler,
		authHandler,
		authMiddleware,
		cfg,
	)

	addr := cfg.ServerPort

	log.Info("http server started", "addres", addr)

	server := &http.Server{Addr: addr, Handler: router, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 70 * time.Second, WriteTimeout: 5 * time.Minute, IdleTimeout: 60 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		log.Error("http server stopped", "error", err)
		os.Exit(1)
	}
}
