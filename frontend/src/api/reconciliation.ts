import { inject, Injectable } from '@angular/core';
import { ApiClient } from './api-client';
import { LegacyBinding, ReconcileReport } from '../types/reconciliation';

@Injectable({ providedIn: 'root' })
export class ReconciliationApi {
  private readonly api = inject(ApiClient);

  reconcile(datasetId: number, itemKey: string) {
    return this.api.post<ReconcileReport>('/reconciliations', { dataset_id: datasetId, item_key: itemKey });
  }

  resume(id: number, note: string) {
    return this.api.post<unknown>(`/reconciliations/suspend-resume/${id}/resume`, { note });
  }

  legacyBindings() {
    return this.api.get<{ unresolved: LegacyBinding[] }>('/legacy-schema-bindings');
  }
}
