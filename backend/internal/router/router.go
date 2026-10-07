package router

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"corpus-annotation-agreement-control/backend/internal/config"
	"corpus-annotation-agreement-control/backend/internal/constants"
	"corpus-annotation-agreement-control/backend/internal/dto"
	"corpus-annotation-agreement-control/backend/internal/handler"
	"corpus-annotation-agreement-control/backend/internal/middleware"
	"corpus-annotation-agreement-control/backend/internal/repository"
	"corpus-annotation-agreement-control/backend/internal/service"
)

type handlers struct {
	system        *handler.SystemHandler
	datasets      *handler.CorpusDatasetHandler
	schemas       *handler.AnnotationSchemaHandler
	annotations   *handler.AnnotationSetHandler
	adjudications *handler.AdjudicationCaseHandler
	backfill      *service.AnnotationSetService
	auth          *service.SystemService
}

func New(db *gorm.DB, cfg config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(middleware.RequestID(), middleware.ErrorHandler(), middleware.Recovery(), middleware.CORS(cfg.CORSOrigin), middleware.RateLimit(cfg.RateLimitPerMinute, "global"), middleware.Audit())
	wired := wire(db, cfg)
	runStartupBackfill(wired)
	engine.GET("/healthz", wired.system.Health)
	engine.GET("/readyz", wired.system.Ready)
	api := engine.Group("/api/v1")
	api.POST("/auth/login", middleware.RateLimit(30, "login"), wired.system.Login)
	protected := api.Group("")
	protected.Use(middleware.Auth(wired.auth))
	registerCorpusDatasetRoutes(protected, wired.datasets)
	registerAnnotationSchemaRoutes(protected, wired.schemas)
	registerAnnotationSetRoutes(protected, wired.annotations)
	registerAdjudicationCaseRoutes(protected, wired.adjudications)
	protected.GET("/audit", middleware.RBAC(constants.RoleAuditor, constants.RoleAdjudicator, constants.RoleAdmin), wired.system.Audit)
	engine.NoRoute(func(context *gin.Context) {
		context.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "route_not_found", "message": "route was not found"}, "request_id": context.GetString("request_id")})
	})
	return engine
}

// runStartupBackfill upgrades historical result sets that predate schema
// version pinning. It is best effort and isolated to the annotation side:
// a backfill failure never blocks the schema center or service startup.
func runStartupBackfill(wired handlers) {
	if wired.backfill == nil {
		return
	}
	report, err := wired.backfill.BackfillSchemaVersions(dto.Actor{ID: 0, Username: "system-startup", Role: constants.RoleAdmin}, "startup-backfill")
	if err != nil {
		slog.Warn("startup schema-version backfill incomplete", "error", err)
		return
	}
	slog.Info("startup schema-version backfill finished",
		"processed_groups", report.ProcessedGroups, "pinned_groups", report.PinnedGroups,
		"pinned_rows", report.PinnedRows, "unresolved_items", report.UnresolvedItems)
}

func wire(db *gorm.DB, cfg config.Config) handlers {
	systemRepository := repository.NewSystemRepository(db)
	datasetRepository := repository.NewCorpusDatasetRepository(db)
	schemaRepository := repository.NewAnnotationSchemaRepository(db)
	annotationRepository := repository.NewAnnotationSetRepository(db)
	adjudicationRepository := repository.NewAdjudicationCaseRepository(db)
	systemService := service.NewSystemService(systemRepository, cfg.JWTSecret, cfg.JWTTTL)
	datasetService := service.NewCorpusDatasetService(db, datasetRepository, systemService)
	schemaService := service.NewAnnotationSchemaService(db, schemaRepository, datasetRepository, systemService)
	annotationService := service.NewAnnotationSetService(db, annotationRepository, datasetRepository, schemaRepository, systemService)
	adjudicationService := service.NewAdjudicationCaseService(db, adjudicationRepository, annotationRepository, systemService, cfg.AlgorithmVersion)
	reconciliationService := service.NewReconciliationService(db, adjudicationRepository, annotationRepository, schemaRepository, datasetRepository, systemService)
	return handlers{
		system: handler.NewSystemHandler(systemService, db), datasets: handler.NewCorpusDatasetHandler(datasetService),
		schemas: handler.NewAnnotationSchemaHandler(schemaService), annotations: handler.NewAnnotationSetHandler(annotationService),
		adjudications: handler.NewAdjudicationCaseHandler(adjudicationService, reconciliationService, systemService),
		backfill:      annotationService, auth: systemService,
	}
}
