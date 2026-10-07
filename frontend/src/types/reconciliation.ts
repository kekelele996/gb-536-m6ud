export interface ReconcileFinding {
  code: string;
  message: string;
  adjudication_id?: number;
  annotation_set_id?: number;
  expected_dataset_code?: string;
  expected_item_key?: string;
}

export interface ReconciledSet {
  id: number;
  annotator: string;
  schema_code: string;
  schema_version: number;
  annotation_state: string;
  schema_binding_state: string;
}

export interface ReconciledCase {
  id: number;
  case_state: string;
  schema_code: string;
  schema_version: number;
  agreement_score: number;
  suspended_reason?: string;
}

export interface ReconcileReport {
  dataset_id: number;
  dataset_code: string;
  item_key: string;
  annotations: ReconciledSet[];
  cases: ReconciledCase[];
  suspended_case_ids: number[];
  findings: ReconcileFinding[];
  matched: boolean;
  reconciled_at: string;
}

export interface LegacyBinding {
  annotation_set_id: number;
  dataset_id: number;
  dataset_code: string;
  item_key: string;
  annotator_id: number;
  reason: string;
  resolved: boolean;
  created_at: string;
}
