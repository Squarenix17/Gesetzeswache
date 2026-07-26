package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/apihttp"
	"github.com/Squarenix17/gesetzeswache/internal/cli"
	"github.com/Squarenix17/gesetzeswache/internal/config"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/httpx"
	"github.com/Squarenix17/gesetzeswache/internal/instruments"
	"github.com/Squarenix17/gesetzeswache/internal/mcp"
	"github.com/Squarenix17/gesetzeswache/internal/metrics"
	"github.com/Squarenix17/gesetzeswache/internal/search"
	"github.com/Squarenix17/gesetzeswache/internal/service"
	"github.com/Squarenix17/gesetzeswache/internal/store"
	"github.com/Squarenix17/gesetzeswache/internal/sync"
)

const version = "0.2.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		log.Error("invalid config", "err", err)
		return 1
	}

	st, err := store.Open(cfg.StorePath)
	if err != nil {
		log.Error("open store", "err", err)
		return 1
	}
	defer st.Close()

	reg := metrics.NewRegistry()
	metrics.RegisterDefaults(reg)

	instrCat, err := instruments.LoadTSV(cfg.LinkedInstrumentsPath)
	if err != nil {
		log.Error("load linked instruments", "err", err)
		return 1
	}
	log.Info("linked instruments loaded", "path", cfg.LinkedInstrumentsPath, "parents", instrCat.Len())

	familyCat, err := instruments.LoadFamiliesTSV(cfg.FortschreibungFamiliesPath)
	if err != nil {
		log.Error("load fortschreibung families", "err", err)
		return 1
	}
	log.Info("fortschreibung families loaded", "path", cfg.FortschreibungFamiliesPath, "parents", familyCat.Len())

	httpClient := httpx.New(cfg.HTTPTimeout, cfg.RequestMinGap, 32<<20)
	httpClient.Metrics = reg
	eng := search.NewEngine()
	if laws, err := st.ListLaws(); err == nil {
		variants, _ := st.ListVariants()
		eng.Swap(laws, variants)
	}
	_ = loadVariantsFile(cfg.VariantsPath, st, eng, log)

	orch := &sync.Orchestrator{CFG: cfg, Store: st, HTTP: httpClient, Search: eng, Log: log, Metrics: reg, Instruments: instrCat, Families: familyCat}
	svc := &service.Service{
		CFG:         cfg,
		Store:       st,
		Search:      eng,
		Sync:        orch,
		HTTP:        httpClient,
		Export:      export.NewCache(cfg.ExportCacheMax),
		Log:         log,
		Metrics:     reg,
		Instruments: instrCat,
		Families:    familyCat,
	}

	if len(args) == 0 {
		args = []string{"serve"}
	}

	switch args[0] {
	case "version":
		fmt.Println(version)
		return 0
	case "serve":
		return serve(cfg, svc, orch, log)
	case "mcp":
		ctx := context.Background()
		if err := mcp.ServeStdio(ctx, svc); err != nil {
			log.Error("mcp", "err", err)
			return 1
		}
		return 0
	default:
		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		return cli.Run(ctx, svc, args)
	}
}

func serve(cfg config.Config, svc *service.Service, orch *sync.Orchestrator, log *slog.Logger) int {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv := &apihttp.Server{Svc: svc, SharedSecret: cfg.SharedSecret, Metrics: svc.Metrics}
	httpSrv := &http.Server{Addr: cfg.HTTPAddr, Handler: srv.Handler()}
	go func() {
		log.Info("http listening", "addr", cfg.HTTPAddr, "version", version)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server", "err", err)
			cancel()
		}
	}()

	go func() {
		cctx, ccancel := context.WithTimeout(ctx, 5*time.Minute)
		log.Info("initial sync starting")
		orch.InitialSync(cctx)
		ccancel()
		log.Info("initial sync finished")
		orch.StartBackground(ctx)
	}()

	<-ctx.Done()
	shutdownCtx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer scancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	log.Info("shutdown complete")
	return 0
}

func loadVariantsFile(path string, st *store.Store, eng *search.Engine, log *slog.Logger) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var variants []domain.LawVariant
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		variants = append(variants, domain.LawVariant{Variant: parts[0], LawID: parts[1]})
	}
	if len(variants) == 0 {
		return nil
	}
	if err := st.ReplaceVariants(variants); err != nil {
		return err
	}
	laws, _ := st.ListLaws()
	eng.Swap(laws, variants)
	log.Info("loaded variants", "count", len(variants))
	return nil
}
