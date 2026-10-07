package constants

import "testing"

func TestAnnotationTransitions(t *testing.T) {
	tests := []struct {
		from, to string
		allowed  bool
	}{
		{AnnotationDraft, AnnotationSubmitted, true},
		{AnnotationSubmitted, AnnotationLocked, true},
		{AnnotationSubmitted, AnnotationReturned, true},
		{AnnotationReturned, AnnotationDraft, true},
		{AnnotationLocked, AnnotationCompared, true},
		{AnnotationCompared, AnnotationSuperseded, true},
		{AnnotationDraft, AnnotationCompared, false},
		{AnnotationSuperseded, AnnotationDraft, false},
	}
	for _, test := range tests {
		if actual := CanTransitionAnnotation(test.from, test.to); actual != test.allowed {
			t.Errorf("%s -> %s = %v, want %v", test.from, test.to, actual, test.allowed)
		}
	}
}

func TestAdjudicationTransitions(t *testing.T) {
	tests := []struct {
		from, to string
		allowed  bool
	}{
		{CaseOpen, CaseAssigned, true},
		{CaseAssigned, CaseAdjudicated, true},
		{CaseAdjudicated, CaseReviewed, true},
		{CaseReviewed, CaseAccepted, true},
		{CaseReviewed, CaseReopened, true},
		{CaseReopened, CaseAssigned, true},
		{CaseOpen, CaseAccepted, false},
		{CaseAccepted, CaseReopened, false},
		{CaseSuspended, CaseAssigned, false},
		{CaseOpen, CaseSuspended, false},
	}
	for _, test := range tests {
		if actual := CanTransitionCase(test.from, test.to); actual != test.allowed {
			t.Errorf("%s -> %s = %v, want %v", test.from, test.to, actual, test.allowed)
		}
	}
}

func TestSuspendedCaseLifecyclePredicates(t *testing.T) {
	if !CaseIsOpen(CaseAssigned) || CaseIsOpen(CaseSuspended) || CaseIsOpen(CaseAccepted) {
		t.Fatal("case open-state predicate is incorrect")
	}
	if !CanResumeSuspendedCase(CaseOpen) || !CanResumeSuspendedCase(CaseReviewed) {
		t.Fatal("resumable suspended states are incorrect")
	}
	if CanResumeSuspendedCase(CaseSuspended) || CanResumeSuspendedCase(CaseAccepted) {
		t.Fatal("suspended and accepted cases must not be resumable targets")
	}
	if !ValidSchemaPinStatus(SchemaPinPinned) || !ValidSchemaPinStatus(SchemaPinLegacy) || !ValidSchemaPinStatus(SchemaPinUnresolved) {
		t.Fatal("schema pin statuses are incomplete")
	}
	if ValidSchemaPinStatus("current") {
		t.Fatal("unexpected schema pin status accepted")
	}
}

func TestDatasetAndSchemaTransitions(t *testing.T) {
	if !CanTransitionDataset(DatasetDraft, DatasetFrozen) || CanTransitionDataset(DatasetDraft, DatasetArchived) {
		t.Fatal("dataset transition rules are incorrect")
	}
	if !CanTransitionSchema(SchemaDraft, SchemaValidated) || !CanTransitionSchema(SchemaValidated, SchemaPublished) {
		t.Fatal("schema transition rules are incorrect")
	}
	if CanTransitionSchema(SchemaDraft, SchemaPublished) {
		t.Fatal("schema publication cannot skip validation")
	}
}
