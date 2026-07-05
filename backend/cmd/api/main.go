package main

import (
	"os"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/ZM854/shopping-manager/backend/internal/database"
	"github.com/ZM854/shopping-manager/backend/internal/logger"
	"github.com/ZM854/shopping-manager/backend/internal/product"
	"github.com/ZM854/shopping-manager/backend/internal/router"
)

func main() {
	cfg := config.Load()

	appEnv := cfg.AppENV

	log := logger.New(appEnv)

	log.Info("starting application", "env", appEnv)

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

	productRepo := product.NewProductRepository(db, log)
	productHandler := product.NewHandler(productRepo, log)

	tokenRepo := auth.NewTokenRepository(db, log)
	tokenService := auth.NewTokenService(
		log, 
		tokenRepo, 
		cfg.JWTAccessSecret, 
		cfg.JWTRefreshSecret,
		cfg.JWTAccessTTL,
		cfg.JWTRefreshTTL,
	)
	
	mailService := auth.NewMailService(log)

	userRepo := auth.NewUserRepository(db, log)
	userService := auth.NewUserService(
		log, 
		userRepo, 
		tokenService,
		mailService,
		"http://localhost" + cfg.ServerPort + "/activate",
	)
	authHandler := auth.NewAuthHandler(log, userService)

	router := router.New(log, productHandler, authHandler)

	addr := cfg.ServerPort

	log.Info("http server started", "addres", addr)

	if err := router.Run(addr); err != nil {
		log.Error("http server stopped", "error", err)
		os.Exit(1)
	}
}
