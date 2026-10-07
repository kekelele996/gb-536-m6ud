package router

import (
	"github.com/gin-gonic/gin"

	"corpus-annotation-agreement-control/backend/internal/constants"
	"corpus-annotation-agreement-control/backend/internal/handler"
	"corpus-annotation-agreement-control/backend/internal/middleware"
)

func registerAdjudicationCaseRoutes(group *gin.RouterGroup, target *handler.AdjudicationCaseHandler) {
	routes := group.Group("/adjudications")
	routes.GET("", target.List)
	routes.GET("/:id", target.Get)
	routes.POST("", middleware.RBAC(constants.RoleDataManager, constants.RoleAdmin),
		middleware.RateLimit(30, "agreement_compute"), target.Compute)
	decisions := routes.Group("")
	decisions.Use(middleware.RBAC(constants.RoleAdjudicator, constants.RoleAdmin))
	decisions.POST("/:id/assign", target.Assign)
	decisions.POST("/:id/decide", middleware.RateLimit(30, "adjudication_submit"), target.Decide)
	decisions.POST("/:id/review", target.Review)
	decisions.POST("/:id/accept", target.Accept)
	decisions.POST("/:id/reopen", target.Reopen)

	// Operations-console reconciliation; it suspends/resumes cases but never
	// publishes or deprecates schemas, which remain schema-center territory.
	manager := group.Group("")
	manager.Use(middleware.RBAC(constants.RoleDataManager, constants.RoleAdmin))
	manager.POST("/reconciliation/run", target.Reconcile)
	manager.GET("/reconciliation/runs", target.ReconciliationRuns)
	manager.POST("/adjudications/:id/resume", target.ResumeSuspended)
}
