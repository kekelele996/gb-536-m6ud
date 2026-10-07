import { inject, Injectable, signal } from '@angular/core';
import { finalize } from 'rxjs';
import { ReconciliationApi } from '../api/reconciliation';
import { LegacyBinding, ReconcileReport } from '../types/reconciliation';
import { apiErrorMessage } from '../utils/api-error';

@Injectable({ providedIn: 'root' })
export class ReconciliationStore {
  private readonly api = inject(ReconciliationApi);
  readonly report = signal<ReconcileReport | null>(null);
  readonly unresolved = signal<LegacyBinding[]>([]);
  readonly loading = signal(false);
  readonly error = signal('');

  reconcile(datasetId: number, itemKey: string): void {
    this.loading.set(true);
    this.error.set('');
    this.api.reconcile(datasetId, itemKey).pipe(finalize(() => this.loading.set(false))).subscribe({
      next: ({ data }) => this.report.set(data),
      error: (error) => this.error.set(apiErrorMessage(error)),
    });
  }

  resume(id: number, note: string, done: () => void): void {
    this.loading.set(true);
    this.error.set('');
    this.api.resume(id, note).pipe(finalize(() => this.loading.set(false))).subscribe({
      next: () => done(),
      error: (error) => this.error.set(apiErrorMessage(error)),
    });
  }

  loadLegacy(): void {
    this.api.legacyBindings().subscribe({
      next: ({ data }) => this.unresolved.set(data.unresolved),
      error: (error) => this.error.set(apiErrorMessage(error)),
    });
  }
}
