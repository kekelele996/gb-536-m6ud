package constants

const (
	RoleAdmin       = "admin"
	RoleDataManager = "data_manager"
	RoleAnnotator   = "annotator"
	RoleAdjudicator = "adjudicator"
	RoleAuditor     = "auditor"

	DatasetDraft    = "draft"
	DatasetFrozen   = "frozen"
	DatasetArchived = "archived"

	SchemaDraft      = "draft"
	SchemaValidated  = "validated"
	SchemaPublished  = "published"
	SchemaDeprecated = "deprecated"

	AnnotationDraft      = "draft"
	AnnotationSubmitted  = "submitted"
	AnnotationReturned   = "returned"
	AnnotationLocked     = "locked"
	AnnotationCompared   = "compared"
	AnnotationSuperseded = "superseded"

	CaseOpen        = "open"
	CaseAssigned    = "assigned"
	CaseAdjudicated = "adjudicated"
	CaseReviewed    = "reviewed"
	CaseAccepted    = "accepted"
	CaseReopened    = "reopened"
	CaseSuspended   = "suspended"

	SchemaPinLegacy     = "legacy"
	SchemaPinPinned     = "pinned"
	SchemaPinUnresolved = "unresolved"
)

func ValidRole(value string) bool {
	switch value {
	case RoleAdmin, RoleDataManager, RoleAnnotator, RoleAdjudicator, RoleAuditor:
		return true
	default:
		return false
	}
}

func ValidAnnotationState(value string) bool {
	switch value {
	case AnnotationDraft, AnnotationSubmitted, AnnotationReturned, AnnotationLocked, AnnotationCompared, AnnotationSuperseded:
		return true
	default:
		return false
	}
}

func CanTransitionDataset(from, to string) bool {
	return (from == DatasetDraft && to == DatasetFrozen) ||
		(from == DatasetFrozen && to == DatasetArchived)
}

func CanTransitionSchema(from, to string) bool {
	return (from == SchemaDraft && to == SchemaValidated) ||
		(from == SchemaValidated && to == SchemaPublished) ||
		(from == SchemaPublished && to == SchemaDeprecated)
}

func CanTransitionAnnotation(from, to string) bool {
	allowed := map[string]map[string]bool{
		AnnotationDraft:     {AnnotationSubmitted: true},
		AnnotationSubmitted: {AnnotationLocked: true, AnnotationReturned: true},
		AnnotationReturned:  {AnnotationDraft: true},
		AnnotationLocked:    {AnnotationCompared: true},
		AnnotationCompared:  {AnnotationSuperseded: true},
	}
	return allowed[from][to]
}

func CanTransitionCase(from, to string) bool {
	allowed := map[string]map[string]bool{
		CaseOpen:        {CaseAssigned: true},
		CaseReopened:    {CaseAssigned: true},
		CaseAssigned:    {CaseAdjudicated: true},
		CaseAdjudicated: {CaseReviewed: true},
		CaseReviewed:    {CaseAccepted: true, CaseReopened: true},
	}
	return allowed[from][to]
}

func ValidSchemaPinStatus(value string) bool {
	switch value {
	case SchemaPinLegacy, SchemaPinPinned, SchemaPinUnresolved:
		return true
	default:
		return false
	}
}

// CaseIsOpen reports whether a case is still awaiting adjudication, as opposed
// to a concluded (accepted) case or a reconciliation-suspended case.
func CaseIsOpen(value string) bool {
	switch value {
	case CaseOpen, CaseAssigned, CaseAdjudicated, CaseReviewed, CaseReopened:
		return true
	default:
		return false
	}
}

// CanResumeSuspendedCase allows reconciliation to return a suspended case to
// the exact state it occupied before the hold, provided that state is still
// actionable.
func CanResumeSuspendedCase(previous string) bool {
	switch previous {
	case CaseOpen, CaseAssigned, CaseAdjudicated, CaseReviewed, CaseReopened:
		return true
	default:
		return false
	}
}
