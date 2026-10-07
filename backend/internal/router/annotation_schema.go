package router

import (
	"github.com/gin-gonic/gin"

	"corpus-annotation-agreement-control/backend/internal/constants"
	"corpus-annotation-agreement-control/backend/internal/handler"
	"corpus-annotation-agreement-control/backend/internal/middleware"
)

func registerAnnotationSchemaRoutes(group *gin.RouterGroup, target *handler.AnnotationSchemaHandler) {
	routes := group.Group("/schemas")
	routes.GET("", target.List)
	routes.GET("/:id", target.Get)
	write := routes.Group("")
	write.Use(middleware.RBAC(constants.RoleDataManager, constants.RoleAdmin))
	write.POST("", target.Create)
	write.PUT("/:id", target.Update)
	write.POST("/:id/copy", target.Copy)
	// The generic transition handles validate; publication and deprecation have
	// dedicated schema-center endpoints with a release ledger and impact report.
	write.POST("/:id/transition", target.Transition)
	write.POST("/:id/publish", target.Publish)
	write.POST("/:id/deprecate", target.Deprecate)
	routes.GET("/:id/publish-attempts", target.PublishAttempts)
}
