package router

import (
	"github.com/gin-gonic/gin"

	"corpus-annotation-agreement-control/backend/internal/constants"
	"corpus-annotation-agreement-control/backend/internal/handler"
	"corpus-annotation-agreement-control/backend/internal/middleware"
)

func registerReconciliationRoutes(group *gin.RouterGroup, target *handler.ReconciliationHandler) {
	group.POST("/reconciliations",
		middleware.RBAC(constants.RoleDataManager, constants.RoleAdjudicator, constants.RoleAdmin),
		target.Reconcile)
	group.POST("/reconciliations/suspend-resume/:id/resume",
		middleware.RBAC(constants.RoleDataManager, constants.RoleAdjudicator, constants.RoleAdmin),
		target.Resume)
	group.GET("/legacy-schema-bindings",
		middleware.RBAC(constants.RoleDataManager, constants.RoleAuditor, constants.RoleAdmin),
		target.LegacyBindings)
}
