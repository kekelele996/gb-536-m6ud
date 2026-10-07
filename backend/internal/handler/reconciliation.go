package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"corpus-annotation-agreement-control/backend/internal/dto"
	"corpus-annotation-agreement-control/backend/internal/service"
)

type ReconciliationHandler struct {
	service *service.ReconciliationService
	upgrade *service.UpgradeService
}

func NewReconciliationHandler(service *service.ReconciliationService, upgrade *service.UpgradeService) *ReconciliationHandler {
	return &ReconciliationHandler{service: service, upgrade: upgrade}
}

// Reconcile compares the schema center and the annotation console by dataset
// code and item key, suspending cases that cannot be matched.
func (handler *ReconciliationHandler) Reconcile(context *gin.Context) {
	var request dto.ReconcileRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	report, err := handler.service.Reconcile(request, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, report)
}

// Resume re-opens a suspended case once reconciliation succeeds.
func (handler *ReconciliationHandler) Resume(context *gin.Context) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	var request dto.ResumeRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	item, err := handler.service.ResumeCase(id, request, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, item)
}

// LegacyBindings lists old result sets whose effective schema version could
// not be backfilled at upgrade time.
func (handler *ReconciliationHandler) LegacyBindings(context *gin.Context) {
	items, err := handler.upgrade.ListUnresolved()
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, gin.H{"unresolved": items})
}
