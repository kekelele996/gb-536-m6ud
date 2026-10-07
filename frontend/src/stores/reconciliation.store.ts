import { inject, Injectable, signal } from '@angular/core';
import { finalize } from 'rxjs';
import { ReconciliationApi } from '../api/reconciliation';
import { BackfillReport, ReconciliationRun } from '../types/adjudication-case';
import { AnnotationSet } from '../types/annotation-set';
import { apiErrorMessage } from '../utils/api-error';

@Injectable({ providedIn: 'root' })
export class ReconciliationStore {
  private readonly api = inject(ReconciliationApi);
  readonly runs = signal<ReconciliationRun[]>([]);
  readonly latest = signal<ReconciliationRun | null>(null);
  readonly unresolved = signal<AnnotationSet[]>([]);
  readonly backfill = signal<BackfillReport | null>(null);
  readonly loading = signal(false);
  readonly error = signal('');

  load(): void {
    this.api.runs().subscribe({
      next: ({ data }) => this.runs.set(data),
      error: (error) => this.error.set(apiErrorMessage(error)),
    });
    this.api.unresolved().subscribe({
      next: ({ data }) => this.unresolved.set(data),
      error: (error) => this.error.set(apiErrorMessage(error)),
    });
  }

  run(datasetId?: number, itemKey?: string, done?: () => void): void {
    this.loading.set(true);
    this.error.set('');
    this.api.run(datasetId, itemKey).pipe(finalize(() => this.loading.set(false))).subscribe({
      next: ({ data }) => {
        this.latest.set(data);
        this.runs.update((runs) => [data, ...runs.filter((run) => run.id !== data.id)]);
        done?.();
      },
      error: (error) => this.error.set(apiErrorMessage(error)),
    });
  }

  resume(caseId: number, note: string, done?: () => void): void {
    this.loading.set(true);
    this.error.set('');
    this.api.resume(caseId, note).pipe(finalize(() => this.loading.set(false))).subscribe({
      next: () => done?.(),
      error: (error) => this.error.set(apiErrorMessage(error)),
    });
  }

  runBackfill(done?: () => void): void {
    this.loading.set(true);
    this.error.set('');
    this.api.backfill().pipe(finalize(() => this.loading.set(false))).subscribe({
      next: ({ data }) => {
        this.backfill.set(data);
        done?.();
      },
      error: (error) => this.error.set(apiErrorMessage(error)),
    });
  }
}
