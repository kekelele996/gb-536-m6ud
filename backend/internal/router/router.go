package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"corpus-annotation-agreement-control/backend/internal/config"
	"corpus-annotation-agreement-control/backend/internal/constants"
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
	reconcile     *handler.ReconciliationHandler
	auth          *service.SystemService
}

func New(db *gorm.DB, cfg config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(middleware.RequestID(), middleware.ErrorHandler(), middleware.Recovery(), middleware.CORS(cfg.CORSOrigin), middleware.RateLimit(cfg.RateLimitPerMinute, "global"), middleware.Audit())
	wired := wire(db, cfg)
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
	registerReconciliationRoutes(protected, wired.reconcile)
	protected.GET("/audit", middleware.RBAC(constants.RoleAuditor, constants.RoleAdjudicator, constants.RoleAdmin), wired.system.Audit)
	engine.NoRoute(func(context *gin.Context) {
		context.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "route_not_found", "message": "route was not found"}, "request_id": context.GetString("request_id")})
	})
	return engine
}

func wire(db *gorm.DB, cfg config.Config) handlers {
	systemRepository := repository.NewSystemRepository(db)
	datasetRepository := repository.NewCorpusDatasetRepository(db)
	schemaRepository := repository.NewAnnotationSchemaRepository(db)
	publishRepository := repository.NewSchemaPublishRepository(db)
	annotationRepository := repository.NewAnnotationSetRepository(db)
	adjudicationRepository := repository.NewAdjudicationCaseRepository(db)
	legacyRepository := repository.NewLegacyBindingRepository(db)
	systemService := service.NewSystemService(systemRepository, cfg.JWTSecret, cfg.JWTTTL)
	datasetService := service.NewCorpusDatasetService(db, datasetRepository, systemService)
	schemaService := service.NewAnnotationSchemaService(db, schemaRepository, publishRepository, datasetRepository, systemService)
	annotationService := service.NewAnnotationSetService(db, annotationRepository, datasetRepository, schemaRepository, systemService)
	adjudicationService := service.NewAdjudicationCaseService(db, adjudicationRepository, annotationRepository, schemaRepository, systemService, cfg.AlgorithmVersion)
	reconciliationService := service.NewReconciliationService(db, datasetRepository, annotationRepository, adjudicationRepository, schemaRepository, systemService)
	upgradeService := service.NewUpgradeService(db, annotationRepository, schemaRepository, legacyRepository)
	return handlers{
		system: handler.NewSystemHandler(systemService, db), datasets: handler.NewCorpusDatasetHandler(datasetService),
		schemas: handler.NewAnnotationSchemaHandler(schemaService), annotations: handler.NewAnnotationSetHandler(annotationService),
		adjudications: handler.NewAdjudicationCaseHandler(adjudicationService),
		reconcile:     handler.NewReconciliationHandler(reconciliationService, upgradeService), auth: systemService,
	}
}
