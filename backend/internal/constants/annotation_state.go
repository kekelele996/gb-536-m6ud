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

	BindingCurrent    = "current"
	BindingPinned     = "pinned"
	BindingUnresolved = "unresolved"
	PublishSucceeded  = "succeeded"
	PublishFailed     = "failed"
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
		CaseOpen:        {CaseAssigned: true, CaseSuspended: true},
		CaseReopened:    {CaseAssigned: true, CaseSuspended: true},
		CaseAssigned:    {CaseAdjudicated: true, CaseSuspended: true},
		CaseAdjudicated: {CaseReviewed: true},
		CaseReviewed:    {CaseAccepted: true, CaseReopened: true},
		CaseSuspended:   {CaseOpen: true},
	}
	return allowed[from][to]
}

// CanSuspendCase reports whether an unresolved reconciliation finding may put
// the case on hold. Decided, reviewed and accepted cases stay on the record.
func CanSuspendCase(state string) bool {
	return CanTransitionCase(state, CaseSuspended)
}
