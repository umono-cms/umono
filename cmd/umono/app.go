package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/template/html/v2"
	umonocrypto "github.com/umono-cms/crypto"
	"github.com/umono-cms/umono"
	"github.com/umono-cms/umono/internal/config"
	"github.com/umono-cms/umono/internal/handler"
	"github.com/umono-cms/umono/internal/handler/middleware"
	"github.com/umono-cms/umono/internal/i18n"
	"github.com/umono-cms/umono/internal/migration"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/repository"
	"github.com/umono-cms/umono/internal/service"
	"github.com/umono-cms/umono/internal/theme"
	"github.com/umono-cms/umono/internal/view"
	"gorm.io/gorm"
)

// newApp migrates db, wires the services and handlers and registers the
// routes. It does not listen.
func newApp(db *gorm.DB, umonoSecret *umonocrypto.Secret, devRoot string) (*fiber.App, error) {
	if err := db.AutoMigrate(
		&models.Component{},
		&models.SiteContext{},
		&models.Contract{},
		&models.SitePage{},
		&models.PageDiagnostic{},
		&models.Option{},
		&models.Storage{},
		&models.Media{},
		&models.MediaVariant{},
		&models.Secret{},
		&models.Job{},
	); err != nil {
		return nil, fmt.Errorf("migrate err: %w", err)
	}

	if err := migration.GlobalParamLine(db); err != nil {
		return nil, fmt.Errorf("global param line migration err: %w", err)
	}
	notFoundPageID, err := migration.NotFoundPage(db)
	if err != nil {
		return nil, fmt.Errorf("not found page migration err: %w", err)
	}

	// TODO: Refactor: move DI another file
	optionRepo := repository.NewOptionRepository(db)
	storageRepo := repository.NewStorageRepository(db)
	mediaRepo := repository.NewMediaRepository(db)
	secretRepo := repository.NewSecretRepository(db)
	jobRepo := repository.NewJobRepository(db)

	sitePageRepo := repository.NewSitePageRepository(db)
	componentRepo := repository.NewComponentRepository(db)
	componentService := service.NewComponentService(componentRepo)
	siteContextService, err := service.NewSiteContextService(repository.NewSiteContextRepository(db))
	if err != nil {
		return nil, fmt.Errorf("site context err: %w", err)
	}

	settingsService := service.NewSettingsService()
	systemService := service.NewSystemService()
	secretService := service.NewSecretService(secretRepo, umonoSecret)
	storageService := service.NewStorageService(storageRepo, secretService)
	bundle, err := i18n.LoadBundle(umono.Locales(), service.DefaultLanguage)
	if err != nil {
		return nil, fmt.Errorf("i18n err: %w", err)
	}
	optionService := service.NewOptionService(optionRepo, bundle)
	mediaService := service.NewMediaService(mediaRepo, storageRepo, optionRepo, secretService, "uploads/.pending")
	if err := mediaService.EnsureDefaultLocalStorage("uploads"); err != nil {
		return nil, fmt.Errorf("media storage err: %w", err)
	}
	contentCompiler, err := service.NewContentCompiler(
		componentService.GetAll(),
		service.NewMediaContextProvider(mediaService),
		siteContextService,
	)
	if err != nil {
		return nil, fmt.Errorf("content compiler err: %w", err)
	}
	componentService.SetContentCompiler(contentCompiler)
	sitePageService := service.NewSitePageService(sitePageRepo, optionRepo, contentCompiler)
	sitePageService.SetPageDiagnosticRepository(repository.NewPageDiagnosticRepository(db))
	recomputeDiagnostics := func(ctx context.Context) {
		if err := sitePageService.RecomputeAllDiagnostics(context.WithoutCancel(ctx)); err != nil {
			log.Printf("recompute page diagnostics: %v", err)
		}
	}
	componentService.OnRenderInputChange(recomputeDiagnostics)
	mediaService.OnRenderInputChange(recomputeDiagnostics)
	siteContextService.OnRenderInputChange(recomputeDiagnostics)
	// themes/ and themes-pending/ are in the project root, on the same file
	// system, next to umono.db.
	themeService := service.NewThemeService("themes", "themes-pending", service.NewContractService(repository.NewContractRepository(db)), repository.NewThemeRepository(db), theme.NewRegistry())
	sitePageService.SetPageThemes(themeService)
	themeService.OnRenderInputChange(recomputeDiagnostics)
	if err := themeService.LoadActive(context.Background()); err != nil {
		log.Printf("error: load active themes: %v", err)
	}
	if devRoot != "" {
		themeService.EnableDev(devRoot)
		if err := themeService.LoadDev(context.Background()); err != nil {
			log.Printf("error: theme in development: %v", err)
		}
	}
	if err := sitePageService.FillDiagnosticsOnce(context.Background()); err != nil {
		log.Printf("fill page diagnostics: %v", err)
	}
	if notFoundPageID != 0 {
		if err := sitePageService.RecomputeDiagnostics(context.Background(), notFoundPageID); err != nil {
			log.Printf("recompute diagnostics of the 404 page: %v", err)
		}
	}

	jobService := service.NewJobService(jobRepo)
	if err := mediaService.RegisterVariantJobHandlers(jobService); err != nil {
		return nil, fmt.Errorf("media variant job handler err: %w", err)
	}
	if err := jobService.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("job service err: %w", err)
	}

	adminHandler := handler.NewAdminHandler(
		sitePageService,
		componentService,
		themeService,
	)

	previewHandler := handler.NewPreviewHandler(sitePageService, componentService)
	fallbackTheme := theme.NewFallback()
	siteHandler := handler.NewSiteHandler(sitePageService, optionService, fallbackTheme.Release)
	themeHandler := handler.NewThemeHandler(fallbackTheme, themeService)
	sitePageHandler := handler.NewSitePageHandler(sitePageService, componentService, themeService)
	componentHandler := handler.NewComponentHandler(componentService, sitePageService)
	mediaHandler := handler.NewMediaHandler(mediaService, storageService, optionService)
	settingsHandler := handler.NewSettingsHandler(settingsService, optionService, sitePageService, storageService, themeService)
	systemHandler := handler.NewSystemHandler(systemService, jobService, sitePageService)
	contextHandler := handler.NewContextHandler(siteContextService)
	optionHandler := handler.NewOptionHandler(optionService)

	shellHandler, err := handler.NewShellHandler(umono.Shell())
	if err != nil {
		return nil, fmt.Errorf("shell err: %w", err)
	}

	engine := html.NewFileSystem(http.FS(umono.Views()), ".html")
	engine.AddFunc("t", view.Translate)

	store := config.NewSessionStore()

	authHandler := handler.NewAuthHandler(store)

	app := fiber.New(fiber.Config{
		Views:     engine,
		BodyLimit: service.LocalStorageImageUploadBodyLimitMB * 1024 * 1024,
		// Request values such as FormValue point into a buffer that the next
		// request reuses. Services keep some of them in memory, for example
		// the globals of the content compiler.
		Immutable: true,
	})

	app.Use(func(c *fiber.Ctx) error {
		c.Set("X-Powered-By", "Umono")
		return c.Next()
	})
	app.Use(middleware.I18nContext(optionService, bundle))

	app.Use("/static", staticCacheControl(umono.Version), filesystem.New(filesystem.Config{
		Root:   http.FS(umono.Public()),
		Browse: false,
	}))

	if os.Getenv("APP_ENV") == "dev" {
		app.Use(func(c *fiber.Ctx) error {
			if c.Get("HX-Request") == "true" {
				time.Sleep(400 * time.Millisecond)
			}
			return c.Next()
		})
	}
	admin := app.Group("/admin")
	admin.Use(middleware.HTMXContext())

	admin.Get("/login", middleware.Guest(store), authHandler.RenderLogin)

	adminProtected := admin.Group("/", middleware.Logged(store))

	adminProtected.Get("/", adminHandler.Index)

	adminProtected.Get("/site-pages/new", sitePageHandler.RenderNewPageSiteEditor)
	adminProtected.Get("/components/new", componentHandler.RenderComponentEditor)

	adminProtected.Get("/site-pages/list",
		middleware.OnlyHTMX(),
		adminHandler.RenderSitePageList,
	)

	adminProtected.Get("/site-pages/layout-info",
		middleware.OnlyHTMX(),
		sitePageHandler.LayoutInfo,
	)
	adminProtected.Get("/site-pages/:id", adminHandler.RenderAdminSitePage)
	adminProtected.Get("/site-pages/:id/editor",
		middleware.OnlyHTMX(),
		adminHandler.RenderAdminSitePageEditor,
	)

	adminProtected.Post("/site-pages",
		middleware.OnlyHTMX(),
		sitePageHandler.Create,
	)

	adminProtected.Put("/site-pages/:id",
		middleware.OnlyHTMX(),
		sitePageHandler.Update,
	)

	adminProtected.Delete("/site-pages/:id",
		middleware.OnlyHTMX(),
		sitePageHandler.Delete,
	)

	adminProtected.Get("/components/list",
		middleware.OnlyHTMX(),
		adminHandler.RenderComponentList,
	)

	adminProtected.Get("/components/:id", adminHandler.RenderAdminComponent)
	adminProtected.Get("/components/:id/editor",
		middleware.OnlyHTMX(),
		adminHandler.RenderAdminComponentEditor,
	)

	adminProtected.Post("/components",
		middleware.OnlyHTMX(),
		componentHandler.Create,
	)

	adminProtected.Put("/components/:id",
		middleware.OnlyHTMX(),
		componentHandler.Update,
	)

	adminProtected.Delete("/components/:id",
		middleware.OnlyHTMX(),
		componentHandler.Delete,
	)

	adminProtected.Post("/site-pages/preview",
		middleware.OnlyHTMX(),
		previewHandler.RenderSitePagePreview,
	)

	adminProtected.Post("/components/preview",
		middleware.OnlyHTMX(),
		previewHandler.RenderComponentPreview,
	)

	adminProtected.Post("/not-found-page/preview",
		middleware.OnlyHTMX(),
		previewHandler.NotFoundPagePreview,
	)

	adminProtected.Get("/settings", settingsHandler.Index)
	adminProtected.Get("/settings/general", settingsHandler.RenderGeneral)
	adminProtected.Get("/settings/404-page", settingsHandler.Render404Page)
	adminProtected.Get("/settings/themes", settingsHandler.RenderThemes)
	adminProtected.Get("/settings/themes/pages", settingsHandler.RenderThemePages)
	adminProtected.Get("/settings/robots-txt", settingsHandler.RenderRobotsTxt)
	adminProtected.Get("/settings/storage", settingsHandler.RenderStorageIndex)
	adminProtected.Get("/settings/storage/new", settingsHandler.RenderStorageNew)
	adminProtected.Get("/settings/storage/:id", settingsHandler.RenderStorageShow)
	adminProtected.Post("/settings/storage",
		middleware.OnlyHTMX(),
		settingsHandler.CreateStorage,
	)
	adminProtected.Put("/settings/storage/:id",
		middleware.OnlyHTMX(),
		settingsHandler.UpdateStorage,
	)
	adminProtected.Post("/settings/storage/:id/test",
		middleware.OnlyHTMX(),
		settingsHandler.TestStorage,
	)
	adminProtected.Delete("/settings/storage/:id",
		middleware.OnlyHTMX(),
		settingsHandler.DeleteStorage,
	)
	adminProtected.Post("/settings/themes/activate", middleware.OnlyHTMX(), settingsHandler.ActivateTheme)
	adminProtected.Post("/settings/themes/deactivate", middleware.OnlyHTMX(), settingsHandler.DeactivateTheme)
	adminProtected.Post("/settings/themes/retry", middleware.OnlyHTMX(), settingsHandler.RetryTheme)
	adminProtected.Post("/settings/themes/pending/apply", middleware.OnlyHTMX(), settingsHandler.ApplyPending)
	adminProtected.Post("/settings/themes/pending/reject", middleware.OnlyHTMX(), settingsHandler.RejectPending)
	adminProtected.Post("/settings/themes/records/delete", middleware.OnlyHTMX(), settingsHandler.DeleteContractRecord)
	adminProtected.Get("/settings/about", settingsHandler.RenderAbout)
	adminProtected.Get("/system", systemHandler.Index)
	adminProtected.Get("/system/jobs", systemHandler.RenderJobs)
	adminProtected.Get("/system/errors", systemHandler.RenderErrors)
	adminProtected.Post("/system/jobs/:id/retry",
		middleware.OnlyHTMX(),
		systemHandler.RetryJob,
	)
	adminProtected.Get("/context", contextHandler.Index)
	adminProtected.Get("/context/new", contextHandler.RenderNew)
	adminProtected.Get("/context/:id", contextHandler.RenderEdit)
	adminProtected.Post("/context",
		middleware.OnlyHTMX(),
		contextHandler.Create,
	)
	adminProtected.Put("/context/:id",
		middleware.OnlyHTMX(),
		contextHandler.Update,
	)
	adminProtected.Delete("/context/:id",
		middleware.OnlyHTMX(),
		contextHandler.Delete,
	)
	adminProtected.Get("/media", mediaHandler.Index)
	adminProtected.Get("/media/new", mediaHandler.RenderNew)
	adminProtected.Get("/media/pending/:token/preview", mediaHandler.ServePending)
	adminProtected.Get("/media/list",
		middleware.OnlyHTMX(),
		mediaHandler.RenderList,
	)
	adminProtected.Get("/media/library",
		middleware.OnlyHTMX(),
		mediaHandler.RenderLibraryList,
	)
	adminProtected.Get("/media/:id", mediaHandler.RenderShow)
	adminProtected.Post("/media",
		middleware.OnlyHTMX(),
		mediaHandler.Upload,
	)
	adminProtected.Post("/media/presign",
		middleware.OnlyHTMX(),
		mediaHandler.PresignUpload,
	)
	adminProtected.Post("/media/complete",
		middleware.OnlyHTMX(),
		mediaHandler.CompleteUpload,
	)
	adminProtected.Post("/media/confirm",
		middleware.OnlyHTMX(),
		mediaHandler.ConfirmUpload,
	)
	adminProtected.Post("/media/cancel",
		middleware.OnlyHTMX(),
		mediaHandler.CancelUpload,
	)
	adminProtected.Put("/media/:id/alias",
		middleware.OnlyHTMX(),
		mediaHandler.UpdateAlias,
	)
	adminProtected.Delete("/media/:id",
		middleware.OnlyHTMX(),
		mediaHandler.Delete,
	)

	adminProtected.Post("/options/language",
		middleware.OnlyHTMX(),
		optionHandler.SaveLanguage,
	)

	adminProtected.Post("/options/not-found-page-option",
		middleware.OnlyHTMX(),
		sitePageHandler.SaveNotFoundPage,
	)

	adminProtected.Post("/options/local-storage-image-upload-limit",
		middleware.OnlyHTMX(),
		optionHandler.SaveLocalStorageImageUploadLimit,
	)

	adminProtected.Post("/options/robots-txt",
		middleware.OnlyHTMX(),
		optionHandler.SaveRobotsTxt,
	)

	adminProtected.Post("/options/site-url",
		middleware.OnlyHTMX(),
		optionHandler.SaveSiteURL,
	)

	adminProtected.Post("/logout",
		middleware.OnlyHTMX(),
		authHandler.Logout,
	)

	app.Post("/login", authHandler.Login)
	app.Get("/uploads/variants/*", mediaHandler.Serve)
	app.Get("/uploads/:filename", mediaHandler.Serve)

	app.Get("/_umono/:file", shellHandler.Serve)
	app.Get("/_theme/:release/*", themeHandler.Serve)
	app.Get("/robots.txt", siteHandler.RenderRobotsTxt)
	app.Get("/sitemap.xml", siteHandler.RenderSitemap)

	app.Get("/:slug?", siteHandler.RenderSitePage)
	return app, nil
}

// staticCacheControl sets the cache of /static. A release caches the files
// for a year: their URLs carry the version, which changes with every
// release. A local build is always "dev", so the browser revalidates them.
func staticCacheControl(version string) fiber.Handler {
	value := "public, max-age=31536000, immutable"
	if version == "dev" {
		value = "no-cache"
	}
	return func(c *fiber.Ctx) error {
		c.Set("Cache-Control", value)
		return c.Next()
	}
}
