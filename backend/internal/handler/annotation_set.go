package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"corpus-annotation-agreement-control/backend/internal/dto"
	"corpus-annotation-agreement-control/backend/internal/service"
)

type AnnotationSetHandler struct{ service *service.AnnotationSetService }

func NewAnnotationSetHandler(service *service.AnnotationSetService) *AnnotationSetHandler {
	return &AnnotationSetHandler{service: service}
}

func (handler *AnnotationSetHandler) List(context *gin.Context) {
	page, pageSize := Pagination(context)
	items, meta, err := handler.service.List(
		page, pageSize, QueryUint(context, "dataset_id"), context.Query("item_key"), context.Query("state"),
	)
	if err != nil {
		WriteError(context, err)
		return
	}
	WritePage(context, items, meta)
}

func (handler *AnnotationSetHandler) Get(context *gin.Context) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	item, err := handler.service.Get(id)
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, item)
}

func (handler *AnnotationSetHandler) Create(context *gin.Context) {
	var request dto.CreateAnnotationSetRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	item, err := handler.service.Create(request, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusCreated, item)
}

func (handler *AnnotationSetHandler) Update(context *gin.Context) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	var request dto.UpdateAnnotationSetRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	item, err := handler.service.Update(id, request, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, item)
}

func (handler *AnnotationSetHandler) Transition(context *gin.Context) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	var request dto.AnnotationTransitionRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	item, err := handler.service.Transition(id, request, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, item)
}

// BackfillPins upgrades historical result sets without a schema-version pin.
// It is an operations-console action and never changes schema center state.
func (handler *AnnotationSetHandler) BackfillPins(context *gin.Context) {
	report, err := handler.service.BackfillSchemaVersions(Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, report)
}

func (handler *AnnotationSetHandler) UnresolvedPins(context *gin.Context) {
	page, pageSize := Pagination(context)
	items, meta, err := handler.service.UnresolvedPage(page, pageSize)
	if err != nil {
		WriteError(context, err)
		return
	}
	WritePage(context, items, meta)
}
