package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"marketlab/internal/server"
)

//go:embed all:dist
var frontend embed.FS

func main() {
	port := 8080
	if value := os.Getenv("PORT"); value != "" {
		parsed, err := server.ParsePort(value)
		if err != nil {
			log.Fatal(err)
		}
		port = parsed
	}

	assets, err := frontendAssets()
	if err != nil {
		log.Fatal(err)
	}
	application := server.New(assets)
	defer application.Close()

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           application.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownContext); err != nil {
			log.Printf("graceful shutdown: %v", err)
		}
	}()

	log.Printf("MarketLab listening on http://localhost:%d", port)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func frontendAssets() (fs.FS, error) {
	if directory := os.Getenv("MARKETLAB_WEB_DIST"); directory != "" {
		assets := os.DirFS(directory)
		if _, err := fs.Stat(assets, "index.html"); err != nil {
			return nil, fmt.Errorf("MARKETLAB_WEB_DIST: %w", err)
		}
		return assets, nil
	}
	return fs.Sub(frontend, "dist")
}
