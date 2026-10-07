package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"corpus-annotation-agreement-control/backend/internal/dto"
	"corpus-annotation-agreement-control/backend/internal/service"
)

type AdjudicationCaseHandler struct {
	service        *service.AdjudicationCaseService
	reconciliation *service.ReconciliationService
	system         *service.SystemService
}

func NewAdjudicationCaseHandler(service *service.AdjudicationCaseService, reconciliation *service.ReconciliationService, system *service.SystemService) *AdjudicationCaseHandler {
	return &AdjudicationCaseHandler{service: service, reconciliation: reconciliation, system: system}
}

func (handler *AdjudicationCaseHandler) List(context *gin.Context) {
	page, pageSize := Pagination(context)
	items, meta, err := handler.service.List(
		page, pageSize, QueryUint(context, "dataset_id"), context.Query("state"),
		context.Query("disagreement_type"), context.Query("cluster_key"),
	)
	if err != nil {
		WriteError(context, err)
		return
	}
	WritePage(context, items, meta)
}

func (handler *AdjudicationCaseHandler) Get(context *gin.Context) {
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

func (handler *AdjudicationCaseHandler) Compute(context *gin.Context) {
	var request dto.ComputeAdjudicationRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	item, reused, err := handler.service.Compute(request, context.GetHeader("Idempotency-Key"), Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	status := http.StatusCreated
	if reused {
		status = http.StatusOK
	}
	WriteData(context, status, item)
}

func (handler *AdjudicationCaseHandler) Assign(context *gin.Context) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	var request dto.AssignCaseRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	item, err := handler.service.Assign(id, request, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, item)
}

func (handler *AdjudicationCaseHandler) Decide(context *gin.Context) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	var request dto.AdjudicateCaseRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	item, _, err := handler.service.Decide(id, request, context.GetHeader("Idempotency-Key"), Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, item)
}

func (handler *AdjudicationCaseHandler) Review(context *gin.Context) {
	handler.reviewAction(context, handler.service.Review)
}

func (handler *AdjudicationCaseHandler) Accept(context *gin.Context) {
	handler.reviewAction(context, handler.service.Accept)
}

func (handler *AdjudicationCaseHandler) Reopen(context *gin.Context) {
	handler.reviewAction(context, handler.service.Reopen)
}

func (handler *AdjudicationCaseHandler) reviewAction(context *gin.Context, action func(uint, dto.CaseReviewRequest, dto.Actor, string) (dto.AdjudicationCaseResponse, error)) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	var request dto.CaseReviewRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	item, err := action(id, request, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, item)
}

func (handler *AdjudicationCaseHandler) Reconcile(context *gin.Context) {
	var request dto.ReconcileRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	run, err := handler.reconciliation.Run(request, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, run)
}

func (handler *AdjudicationCaseHandler) ReconciliationRuns(context *gin.Context) {
	page, pageSize := Pagination(context)
	items, meta, err := handler.system.ReconciliationRuns(page, pageSize)
	if err != nil {
		WriteError(context, err)
		return
	}
	WritePage(context, items, meta)
}

func (handler *AdjudicationCaseHandler) ResumeSuspended(context *gin.Context) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	var request dto.ResumeCaseRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	item, err := handler.reconciliation.Resume(id, request, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, item)
}
