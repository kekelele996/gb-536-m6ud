import { inject, Injectable } from '@angular/core';
import { ApiClient } from './api-client';
import { AnnotationSet, CreateAnnotationSet, UpdateAnnotationSet } from '../types/annotation-set';
import { AnnotationState } from '../types/enums/annotation-state';

@Injectable({ providedIn: 'root' })
export class AnnotationSetApi {
  private readonly api = inject(ApiClient);

  list(datasetId?: number, itemKey?: string) {
    return this.api.page<AnnotationSet>('/annotations', { page_size: 150, dataset_id: datasetId, item_key: itemKey });
  }

  unresolved() {
    return this.api.page<AnnotationSet>('/annotations/unresolved', { page_size: 150 });
  }

  get(id: number) {
    return this.api.get<AnnotationSet>(`/annotations/${id}`);
  }

  create(payload: CreateAnnotationSet) {
    return this.api.post<AnnotationSet>('/annotations', payload);
  }

  update(id: number, payload: UpdateAnnotationSet) {
    return this.api.put<AnnotationSet>(`/annotations/${id}`, payload);
  }

  transition(id: number, targetState: AnnotationState, reason = '', sourceChecksum?: string) {
    return this.api.post<AnnotationSet>(`/annotations/${id}/transition`, {
      target_state: targetState, reason, source_checksum: sourceChecksum,
    });
  }
}
