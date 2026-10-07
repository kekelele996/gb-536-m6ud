import { inject, Injectable } from '@angular/core';
import { ApiClient } from './api-client';
import { AdjudicationCase, BackfillReport, ReconciliationRun } from '../types/adjudication-case';
import { AnnotationSet } from '../types/annotation-set';

@Injectable({ providedIn: 'root' })
export class ReconciliationApi {
  private readonly api = inject(ApiClient);

  run(datasetId?: number, itemKey?: string) {
    return this.api.post<ReconciliationRun>('/reconciliation/run', {
      dataset_id: datasetId ?? 0,
      item_key: itemKey ?? '',
    });
  }

  runs() {
    return this.api.page<ReconciliationRun>('/reconciliation/runs', { page_size: 50 });
  }

  resume(id: number, note: string) {
    return this.api.post<AdjudicationCase>(`/adjudications/${id}/resume`, { note });
  }

  backfill() {
    return this.api.post<BackfillReport>('/annotations/backfill', {});
  }

  unresolved() {
    return this.api.page<AnnotationSet>('/annotations/unresolved', { page_size: 150 });
  }
}
