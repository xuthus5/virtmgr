package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	stdhttp "net/http"
	"path"
	"strings"
	"time"

	"log/slog"

	"virtmgr/internal/domain"
	apitokensapp "virtmgr/internal/modules/apitokens/app"
	apitokenshttp "virtmgr/internal/modules/apitokens/http"
	audithistoryapp "virtmgr/internal/modules/audithistory/app"
	audithistoryhttp "virtmgr/internal/modules/audithistory/http"
	authapp "virtmgr/internal/modules/auth/app"
	authhttp "virtmgr/internal/modules/auth/http"
	backupsapp "virtmgr/internal/modules/backups/app"
	backupshttp "virtmgr/internal/modules/backups/http"
	configversionsapp "virtmgr/internal/modules/configversions/app"
	configversionshttp "virtmgr/internal/modules/configversions/http"
	dataplaneapp "virtmgr/internal/modules/dataplane/app"
	dataplanehttp "virtmgr/internal/modules/dataplane/http"
	eventsapp "virtmgr/internal/modules/events/app"
	eventshttp "virtmgr/internal/modules/events/http"
	guestagentapp "virtmgr/internal/modules/guestagent/app"
	guestagenthttp "virtmgr/internal/modules/guestagent/http"
	hostapp "virtmgr/internal/modules/host/app"
	hosthttp "virtmgr/internal/modules/host/http"
	imagesapp "virtmgr/internal/modules/images/app"
	imageshttp "virtmgr/internal/modules/images/http"
	monitoringapp "virtmgr/internal/modules/monitoring/app"
	monitoringhttp "virtmgr/internal/modules/monitoring/http"
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
	systemapp "virtmgr/internal/modules/system/app"
	systemhttp "virtmgr/internal/modules/system/http"
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
	"virtmgr/internal/platform/dataplane"
	"virtmgr/internal/platform/eventbus"
	"virtmgr/internal/platform/httpx"
	libvirtbackend "virtmgr/internal/platform/libvirt"
	resourcemetrics "virtmgr/internal/platform/metrics"
	"virtmgr/internal/platform/releasecheck"
	"virtmgr/internal/platform/runtimeconfig"
	"virtmgr/internal/platform/sqlite"
	tasksplatform "virtmgr/internal/platform/tasks"
	"virtmgr/internal/platform/tlsreload"
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
	configVersions  *configversionsapp.Service
	monitoring      *monitoringapp.Service
	guestAgent      *guestagentapp.Service
	auditRetention  *runtimeconfig.AuditRetention
	eventBus        *eventbus.Bus
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	restoreStatus, err := sqlite.ApplyPendingDatabaseRestore(ctx, cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("apply pending database restore: %w", err)
	}
	db, err := sqlite.Open(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := sqlite.Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := sqlite.NewStore(db)
	if restoreStatus.State == domain.DatabaseRestoreApplied {
		_ = store.RecordAudit(ctx, domain.AuditEvent{
			OccurredAt: time.Now().UTC(), Actor: "system", Role: domain.RoleAdmin,
			Action: "system.database.restore.apply", TargetKind: "system", TargetID: "database",
			Result: "success", Detail: map[string]any{
				"filename": restoreStatus.Filename, "sizeBytes": restoreStatus.SizeBytes,
				"schemaVersion": restoreStatus.SchemaVersion, "message": restoreStatus.Message,
			},
		})
	}
	if err := store.DeleteExpiredSessions(ctx, time.Now()); err != nil {
		logger.Warn("expired_sessions_cleanup_failed", "error", err)
	}
	if err := store.DeleteLoginAttemptsBefore(ctx, time.Now().Add(-24*time.Hour)); err != nil {
		logger.Warn("login_attempts_cleanup_failed", "error", err)
	}
	if err := store.DeleteExpiredAPITokens(ctx, time.Now()); err != nil {
		logger.Warn("expired_api_tokens_cleanup_failed", "error", err)
	}
	mode, err := runtimeconfig.New(ctx, store, cfg.ReadOnly)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	auditRetention, err := runtimeconfig.NewAuditRetention(ctx, store, 30)
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
		if err := seedDemoImages(ctx, store); err != nil {
			_ = db.Close()
			return nil, err
		}
	} else {
		backend, err = libvirtbackend.New(cfg.LibvirtURI, cfg.HostName, eventBus)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	locks := dataplane.NewLockManager()
	journal := dataplane.NewJournal(store)
	recoveryService := dataplaneapp.New(store, store, backend, store, locks,
		dataplaneapp.WithErrorHandler(func(err error) { logger.Warn("dataplane_recovery_audit_failed", "error", err) }),
	)
	recoveryContext, recoveryCancel := context.WithTimeout(ctx, 30*time.Minute)
	recoverySummary, err := recoveryService.Recover(recoveryContext)
	recoveryCancel()
	if err != nil {
		_ = backend.Close()
		_ = db.Close()
		return nil, fmt.Errorf("recover interrupted data plane operations: %w", err)
	}
	if recoverySummary.Operations > 0 {
		logger.Info("dataplane_recovery_completed",
			"operations", recoverySummary.Operations, "recovered", recoverySummary.Recovered,
			"needs_attention", recoverySummary.NeedsAttention, "failed", recoverySummary.Failed)
	}
	dataplaneHandler := dataplanehttp.New(recoveryService)

	apiTokenService := apitokensapp.New(store, store)
	apiTokensHandler := apitokenshttp.New(apiTokenService)
	authHandler := authhttp.New(authService, cfg.CookieSecure, cfg.SessionTTL, cfg.ReadOnly, authhttp.WithReadOnlyMode(mode), authhttp.WithTokenAuthenticator(apiTokenService))
	overviewHandler := overviewhttp.New(overviewapp.New(backend))
	metricsService := resourcemetrics.New(backend)
	hostHandler := hosthttp.New(hostapp.New(backend, hostapp.WithMetrics(metricsService)))
	monitoringService := monitoringapp.New(store, metricsService, backend, store,
		monitoringapp.WithGuestAgentSource(backend),
		monitoringapp.WithErrorHandler(func(err error) { logger.Warn("monitoring_sampler_failed", "error", err) }),
	)
	monitoringHandler := monitoringhttp.New(monitoringService)
	eventsHandler := eventshttp.New(eventsapp.New(eventBus))
	taskRecorder := tasksplatform.New(store)
	configVersionService := configversionsapp.New(store, backend, store, cfg.ReadOnly,
		configversionsapp.WithReadOnlyMode(mode),
	)
	backupService := backupsapp.New(store, backend, store, taskRecorder, cfg.ReadOnly,
		backupsapp.WithReadOnlyMode(mode),
		backupsapp.WithErrorHandler(func(err error) { logger.Error("backup_scheduler_failed", "error", err) }),
		backupsapp.WithConfigVersionRecorder(configVersionService),
		backupsapp.WithDataPlane(locks, journal),
	)
	backupsHandler := backupshttp.New(backupService)
	tasksHandler := taskshttp.New(tasksapp.New(store,
		tasksapp.WithRetryer(backupService),
		tasksapp.WithCancellationRegistry(taskRecorder),
		tasksapp.WithAuditRecorder(store),
		tasksapp.WithReadOnlyMode(mode),
	))
	configVersionsHandler := configversionshttp.New(configVersionService)
	storageHandler := storagehttp.New(storageapp.New(backend, store, cfg.ReadOnly, storageapp.WithReadOnlyMode(mode), storageapp.WithTaskRecorder(taskRecorder), storageapp.WithDataPlane(locks, journal)))
	networksHandler := networkshttp.New(networksapp.New(backend, store, cfg.ReadOnly, networksapp.WithReadOnlyMode(mode), networksapp.WithPortForwardStore(store)))
	consoleTokens := consoletoken.New(2 * time.Minute)
	vmsHandler := vmshttp.New(vmsapp.New(backend, store, cfg.ReadOnly, vmsapp.WithReadOnlyMode(mode), vmsapp.WithConsoleTokens(consoleTokens), vmsapp.WithTaskRecorder(taskRecorder), vmsapp.WithMetrics(metricsService), vmsapp.WithBackupService(backupService), vmsapp.WithConfigVersionRecorder(configVersionService), vmsapp.WithVMMetadata(store), vmsapp.WithGuestAgentSnapshotRecorder(store), vmsapp.WithGuestAgentDataCleaner(store), vmsapp.WithImages(store, backend), vmsapp.WithImportVolumes(backend), vmsapp.WithISOVolumes(backend), vmsapp.WithHostProvider(backend), vmsapp.WithDataPlane(locks, journal)))
	imagesHandler := imageshttp.New(imagesapp.New(store, backend, store, cfg.ReadOnly, imagesapp.WithReadOnlyMode(mode)))
	guestAgentService := guestagentapp.New(backend, store, cfg.ReadOnly,
		guestagentapp.WithReadOnlyMode(mode),
		guestagentapp.WithPolicyRepository(store),
		guestagentapp.WithTaskRecorder(taskRecorder),
	)
	guestAgentHandler := guestagenthttp.New(guestAgentService)
	templatesHandler := templateshttp.New(templatesapp.New(store, store, cfg.ReadOnly, templatesapp.WithReadOnlyMode(mode), templatesapp.WithImages(store)))
	snapshotPolicyService := snapshotpoliciesapp.New(store, backend, store, cfg.ReadOnly,
		snapshotpoliciesapp.WithReadOnlyMode(mode),
		snapshotpoliciesapp.WithErrorHandler(func(err error) { logger.Error("snapshot_scheduler_failed", "error", err) }),
	)
	snapshotPoliciesHandler := snapshotpolicieshttp.New(snapshotPolicyService)
	auditHistoryHandler := audithistoryhttp.New(audithistoryapp.New(store, audithistoryapp.WithAuditRecorder(store)))
	usersHandler := usershttp.New(usersapp.New(store, store, cfg.ReadOnly, usersapp.WithReadOnlyMode(mode)))
	settingsHandler := settingshttp.New(settingsapp.New(mode, store, settingsapp.WithAuditRetention(auditRetention)))
	var tlsManager *tlsreload.Manager
	if cfg.TLSEnabled() {
		tlsManager, err = tlsreload.New(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			_ = backend.Close()
			_ = db.Close()
			return nil, err
		}
	}
	systemOptions := []systemapp.Option{
		systemapp.WithDatabasePath(cfg.DatabasePath),
		systemapp.WithReleaseProvider(releasecheck.NewGitHubClient()),
	}
	if tlsManager != nil {
		systemOptions = append(systemOptions, systemapp.WithTLSStatus(tlsManager))
	}
	systemHandler := systemhttp.New(systemapp.New(store, store, systemOptions...))
	handler := buildRouter(routerDependencies{
		logger:           logger,
		cfg:              cfg,
		backend:          backend,
		db:               db,
		auth:             authHandler,
		apiTokens:        apiTokensHandler,
		overview:         overviewHandler,
		host:             hostHandler,
		events:           eventsHandler,
		tasks:            tasksHandler,
		storage:          storageHandler,
		networks:         networksHandler,
		vms:              vmsHandler,
		guestAgent:       guestAgentHandler,
		images:           imagesHandler,
		templates:        templatesHandler,
		snapshotPolicies: snapshotPoliciesHandler,
		auditHistory:     auditHistoryHandler,
		users:            usersHandler,
		settings:         settingsHandler,
		system:           systemHandler,
		monitoring:       monitoringHandler,
		backups:          backupsHandler,
		configVersions:   configVersionsHandler,
		dataplane:        dataplaneHandler,
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
	if tlsManager != nil {
		server.TLSConfig = tlsManager.TLSConfig()
	}
	return &App{config: cfg, logger: logger, handler: handler, server: server, backend: backend, db: db, store: store, scheduler: snapshotPolicyService, backupScheduler: backupService, configVersions: configVersionService, monitoring: monitoringService, guestAgent: guestAgentService, auditRetention: auditRetention, eventBus: eventBus}, nil
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
	go a.runConfigVersionLifecycle(ctx)
	go a.scheduler.Run(ctx)
	go a.backupScheduler.Run(ctx)
	go a.monitoring.Run(ctx)
	go a.runGuestAgentHealth(ctx)
	go a.cleanupExpiredSessions(ctx)
	go a.runAuditRetention(ctx)
	scheme := "http"
	if a.config.TLSEnabled() {
		scheme = "https"
	}
	a.logger.Info("virtmgr_started", "version", version.Version, "listen", a.config.Listen, "scheme", scheme, "demo", a.config.DemoMode, "read_only", a.config.ReadOnly)
	var err error
	if a.config.TLSEnabled() {
		err = a.server.ListenAndServeTLS("", "")
	} else {
		err = a.server.ListenAndServe()
	}
	if errors.Is(err, stdhttp.ErrServerClosed) {
		return nil
	}
	return err
}

func (a *App) runGuestAgentHealth(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	prune := func() {
		if err := a.guestAgent.PruneHealthHistory(ctx, time.Now().UTC().Add(-7*24*time.Hour)); err != nil && !errors.Is(err, context.Canceled) {
			a.logger.Warn("guest_agent_health_cleanup_failed", "error", err)
		}
	}
	prune()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.guestAgent.EnsureHealthPolicies(ctx); err != nil && !errors.Is(err, context.Canceled) {
				a.logger.Warn("guest_agent_policy_sync_failed", "error", err)
			}
			changes, err := a.guestAgent.CheckHealth(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				a.logger.Warn("guest_agent_health_check_failed", "error", err)
				continue
			}
			for _, change := range changes {
				severity := "warning"
				kind := "guest_agent_degraded"
				message := "Guest Agent 连续健康检查失败"
				if change.Kind == "recovered" {
					severity = "info"
					kind = "guest_agent_recovered"
					message = "Guest Agent 已恢复连接"
				}
				a.eventBus.Publish(domain.SystemEvent{
					Source: "virtmgr", Kind: kind, Severity: severity, TargetKind: "vm",
					TargetID: change.VMUUID, Message: message,
					Detail: map[string]any{"state": change.State, "failures": change.Failures, "lastError": change.LastError},
				})
			}
		}
	}
}

func (a *App) runAuditRetention(ctx context.Context) {
	cleanup := func(now time.Time) {
		before := now.AddDate(0, 0, -a.auditRetention.Days())
		deleted, err := a.store.DeleteAuditEventsBefore(ctx, before)
		if err != nil {
			a.logger.Warn("audit_retention_cleanup_failed", "error", err)
			return
		}
		if deleted > 0 {
			a.logger.Info("audit_retention_cleanup", "deleted", deleted, "before", before.UTC())
			_ = a.store.RecordAudit(ctx, domain.AuditEvent{
				OccurredAt: time.Now().UTC(), Actor: "system", Role: domain.RoleAdmin,
				Action: "audit.retention.cleanup", TargetKind: "settings", TargetID: "audit_retention",
				Result: "success", Detail: map[string]any{"deleted": deleted, "before": before.UTC().Format(time.RFC3339)},
			})
		}
	}
	cleanup(time.Now().UTC())
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			cleanup(now.UTC())
		case <-ctx.Done():
			return
		}
	}
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
			if err := a.store.DeleteLoginAttemptsBefore(ctx, now.Add(-24*time.Hour)); err != nil {
				a.logger.Warn("login_attempts_cleanup_failed", "error", err)
			}
			if err := a.store.DeleteExpiredAPITokens(ctx, now); err != nil {
				a.logger.Warn("expired_api_tokens_cleanup_failed", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (a *App) runConfigVersionLifecycle(ctx context.Context) {
	_, events, unsubscribe := a.eventBus.Subscribe(0)
	defer unsubscribe()

	baselineCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	if err := a.configVersions.EnsureBaseline(baselineCtx); err != nil {
		a.logger.Warn("vm_config_version_baseline_failed", "error", err)
	}
	cancel()

	pending := make(map[string]time.Time)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if event.TargetKind != "vm" || event.TargetID == "" {
				continue
			}
			if event.Kind != "domain.defined" && event.Kind != "domain.updated" {
				continue
			}
			// Debounce libvirt bursts so a define operation produces at most one
			// external-change capture after internal handlers have run.
			pending[event.TargetID] = time.Now().Add(1500 * time.Millisecond)
		case now := <-ticker.C:
			for uuid, due := range pending {
				if now.Before(due) {
					continue
				}
				delete(pending, uuid)
				captureCtx, captureCancel := context.WithTimeout(ctx, 30*time.Second)
				_, err := a.configVersions.CaptureVM(captureCtx, uuid, domain.VMConfigCapture{
					Source: domain.VMConfigSourceExternal,
					Actor:  "libvirt-event",
					Role:   domain.RoleAdmin,
					Note:   "检测到外部域定义变更后自动采集",
				})
				captureCancel()
				if err != nil {
					a.logger.Warn("vm_config_version_external_capture_failed", "uuid", uuid, "error", err)
				}
			}
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
	apiTokens        *apitokenshttp.Handler
	overview         *overviewhttp.Handler
	host             *hosthttp.Handler
	events           *eventshttp.Handler
	tasks            *taskshttp.Handler
	storage          *storagehttp.Handler
	networks         *networkshttp.Handler
	vms              *vmshttp.Handler
	guestAgent       *guestagenthttp.Handler
	images           *imageshttp.Handler
	templates        *templateshttp.Handler
	snapshotPolicies *snapshotpolicieshttp.Handler
	auditHistory     *audithistoryhttp.Handler
	users            *usershttp.Handler
	settings         *settingshttp.Handler
	system           *systemhttp.Handler
	monitoring       *monitoringhttp.Handler
	backups          *backupshttp.Handler
	configVersions   *configversionshttp.Handler
	dataplane        *dataplanehttp.Handler
	staticFS         fs.FS
}

func buildRouter(deps routerDependencies) stdhttp.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(httpx.RealIP(deps.cfg.TrustedProxies))
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders(deps.cfg.TLSEnabled() || deps.cfg.CookieSecure))
	r.Use(requestLogger(deps.logger))
	r.Use(httpx.RequireAllowedNetworks(deps.cfg.AllowedNetworks))

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
	r.Get("/metrics", deps.host.Prometheus)

	r.Route("/api/v1", func(api chi.Router) {
		deps.auth.Mount(api)
		deps.apiTokens.MountPublic(api)
		api.Group(func(protected chi.Router) {
			protected.Use(deps.auth.RequireSession)
			deps.apiTokens.MountProtected(protected, deps.auth.RequireCSRF)
			deps.overview.Mount(protected)
			deps.host.Mount(protected)
			deps.events.Mount(protected)
			deps.tasks.Mount(protected, deps.auth.RequireCSRF)
			deps.storage.Mount(protected, deps.auth.RequireCSRF)
			deps.networks.Mount(protected, deps.auth.RequireCSRF)
			deps.vms.Mount(protected, deps.auth.RequireCSRF)
			deps.guestAgent.Mount(protected, deps.auth.RequireCSRF)
			deps.images.Mount(protected, deps.auth.RequireCSRF)
			deps.templates.Mount(protected, deps.auth.RequireCSRF)
			deps.snapshotPolicies.Mount(protected, deps.auth.RequireCSRF)
			deps.auditHistory.Mount(protected)
			deps.users.Mount(protected, deps.auth.RequireCSRF)
			deps.settings.Mount(protected, deps.auth.RequireCSRF)
			deps.system.Mount(protected, deps.auth.RequireCSRF)
			deps.monitoring.Mount(protected, deps.auth.RequireCSRF)
			deps.backups.Mount(protected, deps.auth.RequireCSRF)
			deps.configVersions.Mount(protected, deps.auth.RequireCSRF)
			deps.dataplane.Mount(protected, deps.auth.RequireCSRF)
		})
	})
	r.NotFound(spaHandler(deps.staticFS))
	return r
}

func securityHeaders(hstsEnabled bool) func(stdhttp.Handler) stdhttp.Handler {
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "same-origin")
			w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
			w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; object-src 'none'; script-src 'self' https://static.cloudflareinsights.com; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self' https://cloudflareinsights.com ws: wss:; worker-src 'self' blob:")
			if hstsEnabled {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	}
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
