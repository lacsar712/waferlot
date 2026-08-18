package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lacsar712/waferlot/internal/console"
	"github.com/lacsar712/waferlot/internal/fab"
	"github.com/lacsar712/waferlot/internal/restapi"
	"github.com/lacsar712/waferlot/internal/settings"
)

func main() {
	cfg, err := settings.Load()
	if err != nil {
		log.Fatal(err)
	}
	plant, err := fab.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	mux := http.NewServeMux()
	api := &restapi.API{Plant: plant}
	api.Routes(mux)
	mux.Handle("/", console.DirHandler("web"))

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("waferlot listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	plant.StartForwarders(ctx)
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
}
