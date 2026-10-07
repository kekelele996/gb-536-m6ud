package service

import (
	"log/slog"

	"gorm.io/gorm"

	"corpus-annotation-agreement-control/backend/internal/constants"
	"corpus-annotation-agreement-control/backend/internal/dto"
	"corpus-annotation-agreement-control/backend/internal/model"
	"corpus-annotation-agreement-control/backend/internal/repository"
)

// UpgradeService backfills the effective schema version on legacy result sets
// that predate version pinning. Resolution keys on item key; rows that cannot
// be resolved deterministically are listed separately and marked unresolved.
type UpgradeService struct {
	db          *gorm.DB
	annotations *repository.AnnotationSetRepository
	schemas     *repository.AnnotationSchemaRepository
	legacy      *repository.LegacyBindingRepository
}

func NewUpgradeService(db *gorm.DB, annotations *repository.AnnotationSetRepository, schemas *repository.AnnotationSchemaRepository, legacy *repository.LegacyBindingRepository) *UpgradeService {
	return &UpgradeService{db: db, annotations: annotations, schemas: schemas, legacy: legacy}
}

type BackfillReport struct {
	Scanned    int
	Resolved   int
	Unresolved int
}

// BackfillLegacySchemaBindings is idempotent and runs at startup.
func (service *UpgradeService) BackfillLegacySchemaBindings() (BackfillReport, error) {
	rows, err := service.annotations.LegacyRowsMissingVersion()
	if err != nil {
		return BackfillReport{}, err
	}
	report := BackfillReport{Scanned: len(rows)}
	for _, annotation := range rows {
		datasetCode := annotation.Dataset.DatasetCode
		if datasetCode == "" {
			datasetCode = "unknown"
		}
		schema, resolveErr := service.resolveByItemKey(annotation)
		if resolveErr != nil {
			report.Unresolved++
			if recordErr := service.legacy.Upsert(&model.LegacySchemaBinding{
				AnnotationSetID: annotation.ID, DatasetID: annotation.DatasetID, DatasetCode: datasetCode,
				ItemKey: annotation.ItemKey, AnnotatorID: annotation.AnnotatorID,
				Reason: resolveErr.Error(), Resolved: false,
			}); recordErr != nil {
				return report, recordErr
			}
			if markErr := service.annotations.MarkUnresolvedBinding(annotation.ID); markErr != nil {
				slog.Warn("legacy schema binding left unresolved", "annotation_set_id", annotation.ID, "error", markErr)
			}
			continue
		}
		bindingState := constants.BindingPinned
		if schema.SchemaState == constants.SchemaPublished {
			bindingState = constants.BindingCurrent
		}
		if err := service.annotations.BackfillSchemaBinding(annotation.ID, schema.ID, schema.SchemaCode, schema.Version, bindingState); err != nil {
			return report, err
		}
		if err := service.legacy.Upsert(&model.LegacySchemaBinding{
			AnnotationSetID: annotation.ID, DatasetID: annotation.DatasetID, DatasetCode: datasetCode,
			ItemKey: annotation.ItemKey, AnnotatorID: annotation.AnnotatorID,
			Reason: "backfilled by item key", Resolved: true,
		}); err != nil {
			return report, err
		}
		report.Resolved++
	}
	if report.Scanned > 0 {
		slog.Info("legacy schema version backfill completed",
			"scanned", report.Scanned, "resolved", report.Resolved, "unresolved", report.Unresolved)
	}
	return report, nil
}

// resolveByItemKey uses the schema recorded on sibling result sets for the same
// dataset and item key. Ambiguous items (multiple schema versions) are not
// guessed and resolve to an error so they are listed separately.
func (service *UpgradeService) resolveByItemKey(annotation model.AnnotationSet) (model.AnnotationSchema, error) {
	siblings, err := service.annotations.UniqueSchemasForItemKey(annotation.DatasetID, annotation.ItemKey)
	if err != nil {
		return model.AnnotationSchema{}, err
	}
	schemaIDs := map[uint]bool{}
	for _, sibling := range siblings {
		schemaIDs[sibling.SchemaID] = true
	}
	if annotation.SchemaID > 0 {
		schemaIDs[annotation.SchemaID] = true
	}
	if len(schemaIDs) == 1 && annotation.SchemaID > 0 {
		return service.schemas.Get(annotation.SchemaID)
	}
	if len(schemaIDs) == 1 {
		for id := range schemaIDs {
			return service.schemas.Get(id)
		}
	}
	return model.AnnotationSchema{}, &backfillError{message: "multiple schema versions observed for item key; manual resolution required"}
}

// ListUnresolved returns legacy rows that could not be backfilled.
func (service *UpgradeService) ListUnresolved() ([]dto.LegacyBindingResponse, error) {
	resolved := false
	bindings, err := service.legacy.List(&resolved)
	if err != nil {
		return nil, err
	}
	items := make([]dto.LegacyBindingResponse, 0, len(bindings))
	for _, binding := range bindings {
		items = append(items, dto.LegacyBindingResponse{
			AnnotationSetID: binding.AnnotationSetID, DatasetID: binding.DatasetID,
			DatasetCode: binding.DatasetCode, ItemKey: binding.ItemKey,
			AnnotatorID: binding.AnnotatorID, Reason: binding.Reason, Resolved: binding.Resolved,
			CreatedAt: binding.CreatedAt,
		})
	}
	return items, nil
}

type backfillError struct{ message string }

func (err *backfillError) Error() string { return err.message }
