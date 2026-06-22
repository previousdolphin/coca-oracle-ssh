package main

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	bm "github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"

	tea "github.com/charmbracelet/bubbletea"
	gossh "golang.org/x/crypto/ssh"
)

const defaultSkillsBase = "https://raw.githubusercontent.com/previousdolphin/coca-skills/main"

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	port := env("PORT", "23234")
	host := env("HOST", "0.0.0.0")
	hostKeyPath := env("HOST_KEY_PATH", ".ssh/coca_oracle_ed25519")

	// Skills: load the index up front so the menu is ready on first connect.
	store := NewSkills(env("SKILLS_RAW_BASE", defaultSkillsBase))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	if err := store.Load(ctx); err != nil {
		cancel()
		log.Fatalf("load skills index: %v", err)
	}
	cancel()
	log.Printf("loaded %d skills across %d categories", store.Count(), len(store.Categories()))

	// Periodic index refresh so newly-published skills appear without a redeploy.
	go func() {
		t := time.NewTicker(30 * time.Minute)
		defer t.Stop()
		for range t.C {
			c, cn := context.WithTimeout(context.Background(), 20*time.Second)
			if err := store.Load(c); err != nil {
				log.Printf("skills refresh failed: %v", err)
			}
			cn()
		}
	}()

	oracle := NewOracle(os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("ORACLE_MODEL"))
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		log.Printf("WARNING: ANTHROPIC_API_KEY not set — chat replies will error")
	}
	// 6 messages / minute burst, 120 / day, per IP.
	limiter := NewLimiter(6, time.Minute, 120)

	// Stable host key: prefer a base64 ed25519 key from env (Fly secret), else a
	// file path that Wish generates+persists on first run (local dev).
	var opts []ssh.Option
	if b64 := os.Getenv("SSH_HOST_KEY"); b64 != "" {
		pem, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			log.Fatalf("decode SSH_HOST_KEY: %v", err)
		}
		opts = append(opts, wish.WithHostKeyPEM(pem))
	} else {
		opts = append(opts, wish.WithHostKeyPath(hostKeyPath))
	}

	opts = append(opts,
		wish.WithAddress(net.JoinHostPort(host, port)),
		// Anonymous: accept any key. The session is the identity, no account.
		wish.WithPublicKeyAuth(func(ssh.Context, ssh.PublicKey) bool { return true }),
		wish.WithKeyboardInteractiveAuth(func(ssh.Context, gossh.KeyboardInteractiveChallenge) bool { return true }),
		wish.WithMiddleware(
			bm.Middleware(teaHandler(store, oracle, limiter)),
			activeterm.Middleware(), // require an interactive PTY
			logging.Middleware(),
		),
	)

	srv, err := wish.NewServer(opts...)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("CoCA skill terminal listening on %s:%s", host, port)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	<-done
	log.Printf("shutting down…")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
}

func teaHandler(store *Skills, oracle *Oracle, limiter *Limiter) bm.Handler {
	return func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
		ip := remoteIP(s)
		r := bm.MakeRenderer(s)
		m := newModel(r, store, oracle, limiter, ip)
		return m, []tea.ProgramOption{tea.WithAltScreen()}
	}
}

func remoteIP(s ssh.Session) string {
	addr := s.RemoteAddr()
	if addr == nil {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}
