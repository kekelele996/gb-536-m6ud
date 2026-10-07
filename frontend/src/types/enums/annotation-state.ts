export type AnnotationState = 'draft' | 'submitted' | 'returned' | 'locked' | 'compared' | 'superseded';
export type DatasetState = 'draft' | 'frozen' | 'archived';
export type SchemaState = 'draft' | 'validated' | 'published' | 'deprecated';
export type CaseState = 'open' | 'assigned' | 'adjudicated' | 'reviewed' | 'accepted' | 'reopened' | 'suspended';
export type SchemaBindingState = 'current' | 'pinned' | 'unresolved';
export type PublishStatus = 'succeeded' | 'failed';

export const ANNOTATION_STATES: readonly AnnotationState[] = [
  'draft', 'submitted', 'returned', 'locked', 'compared', 'superseded',
];
