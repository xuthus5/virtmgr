package bootstrap

import (
	"context"
	"errors"
	"io/fs"
	stdhttp "net/http"
	"path"
	"strings"
	"time"

	"log/slog"

	audithistoryapp "virtmgr/internal/modules/audithistory/app"
	audithistoryhttp "virtmgr/internal/modules/audithistory/http"
	authapp "virtmgr/internal/modules/auth/app"
	authhttp "virtmgr/internal/modules/auth/http"
	backupsapp "virtmgr/internal/modules/backups/app"
	backupshttp "virtmgr/internal/modules/backups/http"
	eventsapp "virtmgr/internal/modules/events/app"
	eventshttp "virtmgr/internal/modules/events/http"
	hostapp "virtmgr/internal/modules/host/app"
	hosthttp "virtmgr/internal/modules/host/http"
	networksapp "virtmgr/internal/modules/networks/app"
	networkshttp "virtmgr/internal/modules/networks/http"
	overviewapp "virtmgr/internal/modules/overview/app"
	overviewhttp "virtmgr/internal/modules/overview/http"
	settingsapp "virtmgr/internal/modules/settings/app"
	settingshttp "virtmgr/internal/modules/settings/http"
	snapshotpoliciesapp "virtmgr/internal/modules/snapshotpolicies/app"
	snapshotpolicieshttp "virtmgr/internal/modules/snapshotpolicies/http"
	storageapp "virtmgr/internal/modules/storage/app"
	storagehttp "virtmgr/internal/modules/storage/http"
	tasksapp "virtmgr/internal/modules/tasks/app"
	taskshttp "virtmgr/internal/modules/tasks/http"
	templatesapp "virtmgr/internal/modules/templates/app"
	templateshttp "virtmgr/internal/modules/templates/http"
	usersapp "virtmgr/internal/modules/users/app"
	usershttp "virtmgr/internal/modules/users/http"
	vmsapp "virtmgr/internal/modules/vms/app"
	vmshttp "virtmgr/internal/modules/vms/http"
	"virtmgr/internal/platform/config"
	"virtmgr/internal/platform/consoletoken"
	"virtmgr/internal/platform/eventbus"
	"virtmgr/internal/platform/httpx"
	libvirtbackend "virtmgr/internal/platform/libvirt"
	resourcemetrics "virtmgr/internal/platform/metrics"
	"virtmgr/internal/platform/runtimeconfig"
	"virtmgr/internal/platform/sqlite"
	tasksplatform "virtmgr/internal/platform/tasks"
	"virtmgr/internal/version"
	"virtmgr/web"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type App struct {
	config          config.Config
	logger          *slog.Logger
	handler         stdhttp.Handler
	server          *stdhttp.Server
	backend         libvirtbackend.Backend
	db              interface{ Close() error }
	store           *sqlite.Store
	scheduler       *snapshotpoliciesapp.Service
	backupScheduler *backupsapp.Service
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	db, err := sqlite.Open(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := sqlite.Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := sqlite.NewStore(db)
	if err := store.DeleteExpiredSessions(ctx, time.Now()); err != nil {
		logger.Warn("expired_sessions_cleanup_failed", "error", err)
	}
	mode, err := runtimeconfig.New(ctx, store, cfg.ReadOnly)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := store.MarkRunningTasksInterrupted(ctx, time.Now()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := store.MarkRunningBackupsInterrupted(ctx, time.Now()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if cfg.DemoMode {
		if err := seedDemoTasks(ctx, store); err != nil {
			_ = db.Close()
			return nil, err
		}
		if err := seedDemoTemplates(ctx, store); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	authService := authapp.New(store, store, cfg.SessionTTL)
	if err := authService.EnsureAdmin(ctx, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		_ = db.Close()
		return nil, err
	}

	var backend libvirtbackend.Backend
	eventBus := eventbus.New(500)
	if cfg.DemoMode {
		backend = libvirtbackend.NewDemo(eventBus)
	} else {
		backend, err = libvirtbackend.New(cfg.LibvirtURI, cfg.HostName, eventBus)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	authHandler := authhttp.New(authService, cfg.CookieSecure, cfg.SessionTTL, cfg.ReadOnly, authhttp.WithReadOnlyMode(mode))
	overviewHandler := overviewhttp.New(overviewapp.New(backend))
	metricsService := resourcemetrics.New(backend)
	hostHandler := hosthttp.New(hostapp.New(backend, hostapp.WithMetrics(metricsService)))
	eventsHandler := eventshttp.New(eventsapp.New(eventBus))
	tasksHandler := taskshttp.New(tasksapp.New(store))
	taskRecorder := tasksplatform.New(store)
	backupService := backupsapp.New(store, backend, store, taskRecorder, cfg.ReadOnly,
		backupsapp.WithReadOnlyMode(mode),
		backupsapp.WithErrorHandler(func(err error) { logger.Error("backup_scheduler_failed", "error", err) }),
	)
	backupsHandler := backupshttp.New(backupService)
	storageHandler := storagehttp.New(storageapp.New(backend, store, cfg.ReadOnly, storageapp.WithReadOnlyMode(mode), storageapp.WithTaskRecorder(taskRecorder)))
	networksHandler := networkshttp.New(networksapp.New(backend, store, cfg.ReadOnly, networksapp.WithReadOnlyMode(mode)))
	consoleTokens := consoletoken.New(30 * time.Second)
	vmsHandler := vmshttp.New(vmsapp.New(backend, store, cfg.ReadOnly, vmsapp.WithReadOnlyMode(mode), vmsapp.WithConsoleTokens(consoleTokens), vmsapp.WithTaskRecorder(taskRecorder), vmsapp.WithMetrics(metricsService), vmsapp.WithBackupService(backupService)))
	templatesHandler := templateshttp.New(templatesapp.New(store, store, cfg.ReadOnly, templatesapp.WithReadOnlyMode(mode)))
	snapshotPolicyService := snapshotpoliciesapp.New(store, backend, store, cfg.ReadOnly,
		snapshotpoliciesapp.WithReadOnlyMode(mode),
		snapshotpoliciesapp.WithErrorHandler(func(err error) { logger.Error("snapshot_scheduler_failed", "error", err) }),
	)
	snapshotPoliciesHandler := snapshotpolicieshttp.New(snapshotPolicyService)
	auditHistoryHandler := audithistoryhttp.New(audithistoryapp.New(store))
	usersHandler := usershttp.New(usersapp.New(store, store, cfg.ReadOnly, usersapp.WithReadOnlyMode(mode)))
	settingsHandler := settingshttp.New(settingsapp.New(mode, store))
	handler := buildRouter(routerDependencies{
		logger:           logger,
		cfg:              cfg,
		backend:          backend,
		db:               db,
		auth:             authHandler,
		overview:         overviewHandler,
		host:             hostHandler,
		events:           eventsHandler,
		tasks:            tasksHandler,
		storage:          storageHandler,
		networks:         networksHandler,
		vms:              vmsHandler,
		templates:        templatesHandler,
		snapshotPolicies: snapshotPoliciesHandler,
		auditHistory:     auditHistoryHandler,
		users:            usersHandler,
		settings:         settingsHandler,
		backups:          backupsHandler,
		staticFS:         web.FS(),
	})
	server := &stdhttp.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	return &App{config: cfg, logger: logger, handler: handler, server: server, backend: backend, db: db, store: store, scheduler: snapshotPolicyService, backupScheduler: backupService}, nil
}

func (a *App) Handler() stdhttp.Handler { return a.handler }

func (a *App) Run(ctx context.Context) error {
	defer func() {
		if err := a.backend.Close(); err != nil {
			a.logger.Warn("libvirt_close_failed", "error", err)
		}
		if err := a.db.Close(); err != nil {
			a.logger.Warn("sqlite_close_failed", "error", err)
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			a.logger.Warn("http_shutdown_failed", "error", err)
		}
	}()
	go a.scheduler.Run(ctx)
	go a.backupScheduler.Run(ctx)
	go a.cleanupExpiredSessions(ctx)
	scheme := "http"
	if a.config.TLSEnabled() {
		scheme = "https"
	}
	a.logger.Info("virtmgr_started", "version", version.Version, "listen", a.config.Listen, "scheme", scheme, "demo", a.config.DemoMode, "read_only", a.config.ReadOnly)
	var err error
	if a.config.TLSEnabled() {
		err = a.server.ListenAndServeTLS(a.config.TLSCertFile, a.config.TLSKeyFile)
	} else {
		err = a.server.ListenAndServe()
	}
	if errors.Is(err, stdhttp.ErrServerClosed) {
		return nil
	}
	return err
}

func (a *App) cleanupExpiredSessions(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			if err := a.store.DeleteExpiredSessions(ctx, now); err != nil {
				a.logger.Warn("expired_sessions_cleanup_failed", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

type routerDependencies struct {
	logger  *slog.Logger
	cfg     config.Config
	backend libvirtbackend.Backend
	db      interface {
		PingContext(context.Context) error
	}
	auth             *authhttp.Handler
	overview         *overviewhttp.Handler
	host             *hosthttp.Handler
	events           *eventshttp.Handler
	tasks            *taskshttp.Handler
	storage          *storagehttp.Handler
	networks         *networkshttp.Handler
	vms              *vmshttp.Handler
	templates        *templateshttp.Handler
	snapshotPolicies *snapshotpolicieshttp.Handler
	auditHistory     *audithistoryhttp.Handler
	users            *usershttp.Handler
	settings         *settingshttp.Handler
	backups          *backupshttp.Handler
	staticFS         fs.FS
}

func buildRouter(deps routerDependencies) stdhttp.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)
	r.Use(requestLogger(deps.logger))

	r.Get("/healthz", func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	r.Get("/readyz", func(w stdhttp.ResponseWriter, request *stdhttp.Request) {
		status := map[string]string{"libvirt": "ok", "database": "ok"}
		code := stdhttp.StatusOK
		if err := deps.backend.Ping(request.Context()); err != nil {
			status["libvirt"] = "unavailable"
			code = stdhttp.StatusServiceUnavailable
		}
		if err := deps.db.PingContext(request.Context()); err != nil {
			status["database"] = "unavailable"
			code = stdhttp.StatusServiceUnavailable
		}
		httpx.JSON(w, code, status)
	})

	r.Route("/api/v1", func(api chi.Router) {
		deps.auth.Mount(api)
		api.Group(func(protected chi.Router) {
			protected.Use(deps.auth.RequireSession)
			deps.overview.Mount(protected)
			deps.host.Mount(protected)
			deps.events.Mount(protected)
			deps.tasks.Mount(protected)
			deps.storage.Mount(protected, deps.auth.RequireCSRF)
			deps.networks.Mount(protected, deps.auth.RequireCSRF)
			deps.vms.Mount(protected, deps.auth.RequireCSRF)
			deps.templates.Mount(protected, deps.auth.RequireCSRF)
			deps.snapshotPolicies.Mount(protected, deps.auth.RequireCSRF)
			deps.auditHistory.Mount(protected)
			deps.users.Mount(protected, deps.auth.RequireCSRF)
			deps.settings.Mount(protected, deps.auth.RequireCSRF)
			deps.backups.Mount(protected, deps.auth.RequireCSRF)
		})
	})
	r.NotFound(spaHandler(deps.staticFS))
	return r
}

func securityHeaders(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; object-src 'none'; script-src 'self' https://static.cloudflareinsights.com; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self' https://cloudflareinsights.com ws: wss:; worker-src 'self' blob:")
		next.ServeHTTP(w, r)
	})
}

func requestLogger(logger *slog.Logger) func(stdhttp.Handler) stdhttp.Handler {
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			start := time.Now()
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(wrapped, r)
			logger.Info("http_request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.Status(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}

func spaHandler(assets fs.FS) stdhttp.HandlerFunc {
	fileServer := stdhttp.FileServer(stdhttp.FS(assets))
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			stdhttp.NotFound(w, r)
			return
		}
		clean := strings.TrimPrefix(r.URL.Path, "/")
		if clean != "" {
			if file, err := assets.Open(clean); err == nil {
				info, statErr := file.Stat()
				_ = file.Close()
				if statErr == nil && !info.IsDir() {
					if strings.HasPrefix(r.URL.Path, "/assets/") {
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					}
					fileServer.ServeHTTP(w, r)
					return
				}
			}
			if path.Ext(clean) != "" {
				stdhttp.NotFound(w, r)
				return
			}
		}
		clone := r.Clone(r.Context())
		clone.URL.Path = "/"
		w.Header().Set("Cache-Control", "no-cache")
		fileServer.ServeHTTP(w, clone)
	})
}
