export interface SchemaPublishAttempt {
  id: number;
  schema_id: number;
  attempt_no: number;
  status: 'succeeded' | 'failed';
  failure_reason: string;
  attempted_by: number;
  published_at?: string;
  created_at: string;
}
