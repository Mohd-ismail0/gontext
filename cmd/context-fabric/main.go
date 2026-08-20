package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var roles = map[string]bool{
	"serve": true, "worker": true, "connector": true,
	"migrate": true, "bootstrap": true, "doctor": true,
	"backup": true, "restore": true, "reconcile": true,
	"all": true,
}

func main() {
	role := "serve"
	if len(os.Args) > 1 {
		role = strings.TrimSpace(os.Args[1])
	}
	if env := strings.TrimSpace(os.Getenv("CONTEXT_FABRIC_ROLE")); env != "" {
		role = env
	}
	if !roles[role] {
		fmt.Fprintf(os.Stderr, "unknown role %q; want serve|worker|connector|migrate|bootstrap|doctor|backup|restore|reconcile|all\n", role)
		os.Exit(2)
	}

	switch role {
	case "migrate", "bootstrap", "doctor", "backup", "restore", "reconcile":
		fmt.Printf("context-fabric %s: ok (stub)\n", role)
		return
	case "worker", "connector":
		runUntilSignal(role)
	case "serve", "all":
		if err := runHTTP(role); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "serve error: %v\n", err)
			os.Exit(1)
		}
	}
}

func runUntilSignal(role string) {
	fmt.Printf("context-fabric %s: running\n", role)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	fmt.Printf("context-fabric %s: shutdown\n", role)
}

func runHTTP(role string) error {
	addr := os.Getenv("CONTEXT_FABRIC_LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	mux := http.NewServeMux()
	ready := true
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/health/startup", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, _ *http.Request) {
		if !ready {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/system/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"service": "context-fabric",
			"role":    role,
			"version": envOr("CONTEXT_FABRIC_VERSION", "0.0.0-dev"),
		})
	})

	srv := &http.Server{Addr: addr, Handler: mux}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		ready = false
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("context-fabric %s: listening on %s\n", role, addr)
	return srv.ListenAndServe()
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
