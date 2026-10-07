import { inject, Injectable, signal } from '@angular/core';
import { finalize, Observable } from 'rxjs';
import { AnnotationSchemaApi } from '../api/annotation-schema';
import { ApiEnvelope } from '../types/api';
import { AnnotationSchema, CreateAnnotationSchema, UpdateAnnotationSchema } from '../types/annotation-schema';
import { SchemaState } from '../types/enums/annotation-state';
import { SchemaPublishAttempt } from '../types/schema-publish';
import { apiErrorMessage } from '../utils/api-error';

@Injectable({ providedIn: 'root' })
export class AnnotationSchemaStore {
  private readonly api = inject(AnnotationSchemaApi);
  readonly items = signal<AnnotationSchema[]>([]);
  readonly selected = signal<AnnotationSchema | null>(null);
  readonly publishAttempts = signal<SchemaPublishAttempt[]>([]);
  readonly loading = signal(false);
  readonly error = signal('');

  load(datasetId?: number): void {
    this.loading.set(true);
    this.error.set('');
    this.api.list(datasetId).pipe(finalize(() => this.loading.set(false))).subscribe({
      next: ({ data }) => {
        this.items.set(data);
        const selected = this.selected();
        this.selected.set(data.find((item) => item.id === selected?.id) ?? data[0] ?? null);
      },
      error: (error) => this.error.set(apiErrorMessage(error)),
    });
  }

  choose(schema: AnnotationSchema): void {
    this.selected.set(schema);
    this.loadPublishAttempts(schema.id);
  }

  create(payload: CreateAnnotationSchema, done?: () => void): void {
    this.mutate(this.api.create(payload), done);
  }

  update(id: number, payload: UpdateAnnotationSchema, done?: () => void): void {
    this.mutate(this.api.update(id, payload), done);
  }

  copy(schema: AnnotationSchema, version: number): void {
    this.mutate(this.api.copy(schema.id, version));
  }

  transition(schema: AnnotationSchema, target: SchemaState): void {
    this.mutate(this.api.transition(schema.id, target), undefined, () => this.loadPublishAttempts(schema.id));
  }

  publish(schema: AnnotationSchema, failureInjection = ''): void {
    this.mutate(this.api.publish(schema.id, failureInjection), undefined, () => this.loadPublishAttempts(schema.id));
  }

  deprecate(schema: AnnotationSchema): void {
    this.loading.set(true);
    this.error.set('');
    this.api.deprecate(schema.id).pipe(finalize(() => this.loading.set(false))).subscribe({
      next: ({ data }) => {
        this.items.update((items) =>
          [data.schema, ...items.filter((item) => item.id !== data.schema.id)]
            .sort((left, right) => left.schema_code.localeCompare(right.schema_code) || right.version - left.version),
        );
        this.selected.set(data.schema);
        this.loadPublishAttempts(schema.id);
      },
      error: (error) => this.error.set(apiErrorMessage(error)),
    });
  }

  loadPublishAttempts(schemaId: number): void {
    this.api.publishAttempts(schemaId).subscribe({
      next: ({ data }) => this.publishAttempts.set(data.attempts),
      error: () => this.publishAttempts.set([]),
    });
  }

  private mutate(request: Observable<ApiEnvelope<AnnotationSchema>>, done?: () => void, after?: () => void): void {
    this.loading.set(true);
    this.error.set('');
    request.pipe(finalize(() => this.loading.set(false))).subscribe({
      next: ({ data }) => {
        this.items.update((items) =>
          [data, ...items.filter((item) => item.id !== data.id)]
            .sort((left, right) => left.schema_code.localeCompare(right.schema_code) || right.version - left.version),
        );
        this.selected.set(data);
        done?.();
        after?.();
      },
      error: (error) => this.error.set(apiErrorMessage(error)),
    });
  }
}
