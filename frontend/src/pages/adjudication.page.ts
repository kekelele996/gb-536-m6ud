import { ChangeDetectionStrategy, Component, computed, inject, OnInit, signal } from '@angular/core';
import { DatePipe, DecimalPipe, PercentPipe } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatSelectModule } from '@angular/material/select';
import { LucideAngularModule } from 'lucide-angular';
import { AgreementMatrixComponent } from '../components/common/agreement-matrix.component';
import { AnnotationStateBadgeComponent } from '../components/common/annotation-state-badge.component';
import { DiffEvidenceDrawerComponent } from '../components/common/diff-evidence-drawer.component';
import { useAdjudicationQueue } from '../hooks/use-adjudication-queue';
import { useAuth } from '../hooks/use-auth';
import { AnnotationSchemaStore } from '../stores/annotation-schema.store';
import { AnnotationSetStore } from '../stores/annotation-set.store';
import { CorpusDatasetStore } from '../stores/corpus-dataset.store';
import { AdjudicationCase } from '../types/adjudication-case';
import { AnnotationLabel, AnnotationSet } from '../types/annotation-set';
import { ReconciliationStore } from '../stores/reconciliation.store';

@Component({
  standalone: true,
  imports: [DatePipe, DecimalPipe, PercentPipe, FormsModule, MatButtonModule, MatFormFieldModule, MatInputModule, MatSelectModule, LucideAngularModule, AgreementMatrixComponent, AnnotationStateBadgeComponent, DiffEvidenceDrawerComponent],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="page-head"><div><span>Independent disagreement resolution</span><h1>Adjudication queue</h1></div><div class="head-actions"><button mat-stroked-button type="button" (click)="reload()" [disabled]="cases.loading()"><lucide-icon name="refresh-cw" [size]="16" />Refresh</button></div></section>
    @if (cases.error() || localError()) { <p class="error-banner"><lucide-icon name="triangle-alert" [size]="15" />{{ localError() || cases.error() }}</p> }
    @if (canCompute()) {
      <section class="compute-band"><mat-form-field appearance="outline"><mat-label>Dataset</mat-label><mat-select [(ngModel)]="computeDatasetId">@for (dataset of datasets.items(); track dataset.id) { <mat-option [value]="dataset.id">{{ dataset.dataset_code }}</mat-option> }</mat-select></mat-form-field><mat-form-field appearance="outline"><mat-label>Item key</mat-label><input matInput [(ngModel)]="computeItemKey" /></mat-form-field><mat-form-field appearance="outline"><mat-label>Annotation IDs</mat-label><input matInput [(ngModel)]="computeAnnotationIds" /></mat-form-field><mat-form-field appearance="outline"><mat-label>Metric</mat-label><mat-select [(ngModel)]="computeMetric"><mat-option value="auto">Auto</mat-option><mat-option value="cohen_kappa">Cohen's Kappa</mat-option><mat-option value="krippendorff_alpha">Krippendorff's Alpha</mat-option></mat-select></mat-form-field><button mat-flat-button color="primary" type="button" (click)="compute()" [disabled]="cases.loading()"><lucide-icon name="git-compare-arrows" [size]="16" />Compute</button></section>
      <section class="reconcile-band">
        <span>Schema center / console reconciliation by dataset code + item key</span>
        <div><button mat-stroked-button type="button" (click)="reconcile()" [disabled]="reconciliation.loading() || !computeDatasetId || !computeItemKey"><lucide-icon name="scale" [size]="15" />Reconcile this key</button><button mat-stroked-button type="button" (click)="reconciliation.loadLegacy()" [disabled]="reconciliation.loading()">List unresolved legacy rows</button></div>
        @if (reconciliation.report(); as report) { <article [class.bad]="!report.matched" [class.good]="report.matched"><strong>{{ report.matched ? 'Matched' : 'Mismatch — pending cases suspended' }}</strong><p>{{ report.dataset_code }} / {{ report.item_key }} · {{ report.findings.length }} finding(s) · suspended ids: {{ report.suspended_case_ids.length ? report.suspended_case_ids.join(', ') : 'none' }}</p>@for (finding of report.findings; track finding.code + finding.message) { <small>{{ finding.code }}: {{ finding.message }}</small> }</article> }
        @if (reconciliation.unresolved().length) { <article class="legacy"><strong>{{ reconciliation.unresolved().length }} legacy row(s) could not be backfilled</strong>@for (binding of reconciliation.unresolved(); track binding.annotation_set_id) { <small>#{{ binding.annotation_set_id }} {{ binding.dataset_code }} / {{ binding.item_key }} — {{ binding.reason }}</small> }</article> }
      </section>
    }
    <section class="case-layout">
      <div class="queue">
        <header><span>Cases</span><small>{{ cases.items().length }} in view</small></header>
        @for (item of cases.items(); track item.id) {
          <button type="button" class="case-row" [class.selected]="cases.selected()?.id === item.id" (click)="choose(item)">
            <span class="case-id">#{{ item.id }}</span><span><strong>{{ item.item_key }}</strong><small>{{ item.disagreement_type }} / {{ item.cluster_key }}</small></span><strong class="score">{{ item.agreement.score | number:'1.2-2' }}</strong><app-annotation-state-badge [state]="item.case_state" />
          </button>
        } @empty { <p class="empty">No disagreement cases available</p> }
      </div>
      <section class="evidence-panel">
        @if (cases.selected(); as item) {
          <header><div><span>Case #{{ item.id }} / {{ item.algorithm_version }}</span><h2>{{ item.dataset_code }} / {{ item.item_key }}</h2></div><app-annotation-state-badge [state]="item.case_state" /></header>
          <section class="schema-version-strip"><article><span>Score schema version</span><strong>{{ item.schema_code || '—' }} v{{ item.schema_version || 0 }}</strong></article><article [class.deprecated]="!item.schema_current"><span>Version status</span><strong>{{ item.schema_current ? 'current published' : 'not current (deprecated or missing)' }}</strong></article></section>
          @if (item.case_state === 'suspended') { <section class="suspended-banner"><lucide-icon name="pause-circle" [size]="16" /><div><strong>Case suspended pending reconciliation</strong><p>{{ item.suspended_reason }}</p></div>@if (canManageCase()) { <button mat-flat-button color="primary" type="button" (click)="resumeCase(item)" [disabled]="cases.loading()">Resume after reconcile</button> }</section> }
          <section class="metric-strip"><article><span>Metric</span><strong>{{ metricName(item.agreement.metric) }}</strong></article><article><span>Score</span><strong>{{ item.agreement.score | number:'1.3-3' }}</strong></article><article><span>Observed</span><strong>{{ item.agreement.observed_agreement | percent:'1.1-1' }}</strong></article><article><span>Chance</span><strong>{{ item.agreement.chance_agreement | percent:'1.1-1' }}</strong></article><article><span>Sample / coders</span><strong>{{ item.agreement.sample_size }} / {{ item.agreement.coder_count }}</strong></article></section>
          <aside class="applicability"><lucide-icon name="file-json-2" [size]="16" /><span><strong>{{ item.disagreement_type }} cluster</strong>{{ item.agreement.applicability }}</span><code>{{ item.cluster_key }}</code></aside>
          <section class="snapshot-grid"><div><span>Compared result sets</span>@for (annotation of selectedAnnotations(); track annotation.id) { <article><header><strong>#{{ annotation.id }} / {{ annotation.annotator }}</strong><app-annotation-state-badge [state]="annotation.annotation_state" /></header>@for (label of annotation.labels; track $index) { <p><span>{{ label.unit_key }}</span><b>{{ label.label }}</b><code>{{ interval(label) }}</code></p> }</article> }</div><app-agreement-matrix [cells]="item.confusion_snapshot" /></section>
          <app-diff-evidence-drawer [evidence]="item.evidence_snapshot" />
          @if (canAdjudicate()) {
            <section class="decision-panel">
              <header><span>Controlled state action</span><small>Self-adjudication and same-person review are rejected by the API</small></header>
              @if ((item.case_state === 'open' || item.case_state === 'reopened' || item.case_state === 'assigned') && canManageCase()) { <div class="action-line warn-line"><mat-form-field appearance="outline"><mat-label>Suspension reason</mat-label><input matInput [(ngModel)]="suspendReason" /></mat-form-field><button mat-stroked-button class="danger-button" type="button" (click)="suspendCase(item)" [disabled]="cases.loading()"><lucide-icon name="pause-circle" [size]="16" />Suspend pending reconcile</button></div> }
              @if ((item.case_state === 'open' || item.case_state === 'reopened') && canClaim()) { <div class="action-line"><mat-form-field appearance="outline"><mat-label>Assignment note</mat-label><input matInput [(ngModel)]="actionNote" /></mat-form-field><button mat-flat-button color="primary" type="button" (click)="cases.assign(item, actionNote)" [disabled]="cases.loading()"><lucide-icon name="user-check" [size]="16" />Claim case</button></div> }
              @if (item.case_state === 'assigned' && canDecide(item)) { <div class="decision-form"><div class="decision-tools"><button mat-stroked-button type="button" (click)="useLeftLabels()" [disabled]="cases.loading()">Use left labels</button><span>Final labels remain structural; raw text is never stored here.</span></div><mat-form-field appearance="outline"><mat-label>Final labels (JSON)</mat-label><textarea matInput rows="7" [(ngModel)]="finalLabelsText"></textarea></mat-form-field><mat-form-field appearance="outline"><mat-label>Rationale</mat-label><textarea matInput rows="3" [(ngModel)]="rationale"></textarea></mat-form-field><button mat-flat-button color="primary" type="button" (click)="decide(item)" [disabled]="cases.loading()"><lucide-icon name="gavel" [size]="16" />Record decision</button></div> }
              @if (item.case_state === 'adjudicated' && canReview(item)) { <div class="action-line"><mat-form-field appearance="outline"><mat-label>Independent review note</mat-label><input matInput [(ngModel)]="actionNote" /></mat-form-field><button mat-flat-button color="primary" type="button" (click)="cases.review(item, actionNote)" [disabled]="cases.loading()"><lucide-icon name="check" [size]="16" />Mark reviewed</button></div> }
              @if (item.case_state === 'reviewed' && canConclude(item)) { <div class="action-line"><mat-form-field appearance="outline"><mat-label>Acceptance or reopen note</mat-label><input matInput [(ngModel)]="actionNote" /></mat-form-field><button mat-stroked-button type="button" (click)="cases.reopen(item, actionNote)" [disabled]="cases.loading()">Reopen</button><button mat-flat-button color="primary" type="button" (click)="cases.accept(item, actionNote)" [disabled]="cases.loading()">Accept</button></div> }
            </section>
          }
        } @else { <p class="empty">Select a case to inspect its frozen comparison evidence</p> }
      </section>
    </section>
  `,
  styles: [`
    .compute-band{display:grid;grid-template-columns:.8fr 1fr 1fr .8fr auto;gap:9px;align-items:start;margin-bottom:16px;padding:11px;background:#e7ecea;border:1px solid #c3cdca;border-radius:4px}.compute-band button{height:40px;display:flex;gap:6px}.reconcile-band{display:grid;gap:9px;margin-bottom:16px;padding:11px;background:#f0f3f1;border:1px solid #c3cdca;border-radius:4px}.reconcile-band>span{font-size:10px;text-transform:uppercase;color:#5d6a6f;font-weight:750}.reconcile-band>div{display:flex;gap:8px;flex-wrap:wrap}.reconcile-band article{display:grid;gap:3px;padding:9px 10px;border-left:3px solid #458063;background:#edf6f0}.reconcile-band article.bad{background:#f9ecea;border-left-color:#95332d}.reconcile-band article strong{font-size:11px}.reconcile-band article p{margin:0;font-size:10px;color:#59666a}.reconcile-band article small{font-size:9px;color:#842f2a}.reconcile-band article.legacy{background:#fff7dd;border-left-color:#b8862b}.reconcile-band article.legacy small{color:#725313}.schema-version-strip{display:grid;grid-template-columns:1fr 1fr;background:#e8edeb;border-bottom:1px solid #cbd3d0}.schema-version-strip article{padding:9px 14px}.schema-version-strip article:first-child{border-right:1px solid #c5cecb}.schema-version-strip span{display:block;color:#69767a;font-size:9px;text-transform:uppercase}.schema-version-strip strong{font-size:12px}.schema-version-strip article.deprecated strong{color:#842f2a}.suspended-banner{display:flex;align-items:flex-start;gap:10px;margin:12px 14px 0;padding:10px 12px;background:#f9ecea;border-left:3px solid #95332d}.suspended-banner div{flex:1;display:grid;gap:3px}.suspended-banner strong{font-size:11px}.suspended-banner p{margin:0;font-size:10px;color:#59666a}.warn-line{background:#fff7dd}.danger-button{color:#95332d!important}.case-layout{display:grid;grid-template-columns:minmax(350px,.58fr) minmax(620px,1.42fr);gap:16px;align-items:start}.queue,.evidence-panel{background:#fbfcfa;border:1px solid #c1cbc8;border-radius:4px;overflow:hidden}.queue>header{display:flex;justify-content:space-between;padding:11px 13px;background:#e7ecea;border-bottom:1px solid #c8d1ce}.queue header span{font-size:11px;font-weight:750;text-transform:uppercase}.queue header small{font-size:10px}.case-row{width:100%;display:grid;grid-template-columns:32px minmax(0,1fr) 42px auto;align-items:center;gap:9px;padding:11px;background:#fbfcfa;border:0;border-bottom:1px solid #dce2e0;text-align:left;cursor:pointer}.case-row:hover,.case-row.selected{background:#fff4cf}.case-id{font-size:10px;font-weight:800}.case-row>span:nth-child(2){display:grid;gap:3px;min-width:0}.case-row>span strong{font-size:11px}.case-row small{overflow:hidden;color:#6a777b;font-size:9px;text-overflow:ellipsis;white-space:nowrap}.score{height:31px;display:grid;place-items:center;color:#f3f6f4;background:#435157;border-radius:3px;font-size:10px}.evidence-panel>header{display:flex;align-items:flex-start;justify-content:space-between;gap:10px;padding:14px 15px;border-bottom:1px solid #cad2cf}.evidence-panel header span{color:#69767a;font-size:9px;text-transform:uppercase}.evidence-panel h2{margin:3px 0 0;font-size:17px}.metric-strip{display:grid;grid-template-columns:1.25fr repeat(4,1fr);background:#e8edeb;border-bottom:1px solid #cbd3d0}.metric-strip article{min-width:0;padding:10px 12px;border-right:1px solid #c5cecb}.metric-strip article:last-child{border-right:0}.metric-strip span{display:block;color:#69767a;font-size:9px;text-transform:uppercase}.metric-strip strong{display:block;overflow:hidden;margin-top:4px;font-size:14px;text-overflow:ellipsis;white-space:nowrap}.applicability{display:grid;grid-template-columns:auto minmax(0,1fr) auto;align-items:center;gap:9px;margin:14px;padding:10px;background:#edf2ef;border-left:3px solid #458063}.applicability>span{display:grid;gap:2px;font-size:10px;line-height:1.4}.applicability strong{font-size:9px;text-transform:uppercase}.applicability code{max-width:240px;overflow:hidden;font-size:9px;text-overflow:ellipsis}.snapshot-grid{display:grid;grid-template-columns:minmax(280px,.8fr) minmax(320px,1.2fr);gap:14px;margin:14px}.snapshot-grid>div>span{display:block;margin-bottom:7px;color:#69767a;font-size:9px;text-transform:uppercase}.snapshot-grid>div>article{margin-bottom:8px;border:1px solid #d3dbd8}.snapshot-grid article>header{display:flex;align-items:center;justify-content:space-between;gap:7px;padding:7px 8px;background:#edf1ef}.snapshot-grid article>header strong{font-size:10px}.snapshot-grid article p{display:grid;grid-template-columns:minmax(0,1fr) auto auto;gap:8px;margin:0;padding:6px 8px;border-top:1px solid #e0e5e3;font-size:9px}.snapshot-grid article p b{color:#70510d}.snapshot-grid article code{font-size:9px}.evidence-panel>app-diff-evidence-drawer{display:block;margin:14px}.decision-panel{margin:14px;border-top:1px solid #d3dad8}.decision-panel>header{display:flex;justify-content:space-between;gap:8px;padding:11px 0}.decision-panel>header span{font-size:10px;font-weight:750;text-transform:uppercase}.decision-panel>header small{color:#6a777a;font-size:9px}.action-line{display:flex;align-items:start;gap:8px;padding:10px;background:#eef2f0}.action-line mat-form-field{flex:1}.action-line button{height:40px}.decision-form{display:grid;gap:4px;padding:11px;background:#eef2f0}.decision-tools{display:flex;align-items:center;justify-content:space-between;gap:9px;margin-bottom:7px}.decision-tools span{color:#647176;font-size:9px}.decision-form>button{justify-self:end;display:flex;gap:6px}
    @media(max-width:1180px){.case-layout{grid-template-columns:1fr}.compute-band{grid-template-columns:repeat(2,1fr)}.compute-band button{grid-column:2}}@media(max-width:760px){.compute-band,.snapshot-grid,.metric-strip{grid-template-columns:1fr}.compute-band button{grid-column:1}.case-row{grid-template-columns:30px minmax(0,1fr) 40px}.case-row app-annotation-state-badge{grid-column:2}.metric-strip article{border-right:0;border-bottom:1px solid #c5cecb}.applicability{grid-template-columns:auto minmax(0,1fr)}.applicability code{grid-column:2;max-width:100%}.action-line,.decision-tools{align-items:stretch;flex-direction:column}.decision-panel>header{display:grid}.case-layout{display:block}.evidence-panel{margin-top:12px}}
  `],
})
export class AdjudicationPage implements OnInit {
  readonly datasets = inject(CorpusDatasetStore);
  readonly schemas = inject(AnnotationSchemaStore);
  readonly annotations = inject(AnnotationSetStore);
  readonly cases = useAdjudicationQueue();
  readonly reconciliation = inject(ReconciliationStore);
  readonly auth = useAuth();
  readonly localError = signal('');
  readonly selectedAnnotations = computed<AnnotationSet[]>(() => {
    const ids = this.cases.selected()?.annotation_set_ids ?? [];
    return ids.flatMap((id) => {
      const annotation = this.annotations.items().find((candidate) => candidate.id === id);
      return annotation ? [annotation] : [];
    });
  });
  computeDatasetId = 0;
  computeItemKey = 'DOC-7F2A';
  computeAnnotationIds = '1, 2';
  computeMetric: 'auto' | 'cohen_kappa' | 'krippendorff_alpha' = 'auto';
  actionNote = 'Independent evidence review completed.';
  suspendReason = 'Dataset code and item key do not reconcile against console result sets.';
  finalLabelsText = '[]';
  rationale = 'Resolved against the published schema and frozen structural evidence.';

  canCompute = () => this.auth.can('data_manager', 'admin');
  canManageCase = (): boolean => this.auth.can('data_manager', 'adjudicator', 'admin');
  reconcile(): void {
    if (!this.computeDatasetId || this.computeItemKey.trim().length < 2) return;
    this.reconciliation.reconcile(this.computeDatasetId, this.computeItemKey.trim());
  }
  suspendCase(item: AdjudicationCase): void {
    if (this.suspendReason.trim().length < 8) { this.localError.set('Provide a suspension reason of at least 8 characters.'); return; }
    this.localError.set('');
    this.cases.suspend(item, this.suspendReason.trim());
  }
  resumeCase(item: AdjudicationCase): void {
    this.reconciliation.resume(item.id, 'Dataset code and item key reconciled; returning the case to the open queue.', () => this.cases.load(this.computeDatasetId || undefined));
  }
  canAdjudicate = () => this.auth.can('adjudicator', 'admin');
  canClaim = (): boolean => this.canAdjudicate() && !this.selectedAnnotations().some((annotation) => annotation.annotator_id === this.auth.user()?.id);
  canDecide = (item: AdjudicationCase): boolean => this.canAdjudicate() && item.adjudicator_id === this.auth.user()?.id && this.canClaim();
  canReview = (item: AdjudicationCase): boolean => this.canAdjudicate() && item.adjudicator_id !== this.auth.user()?.id && this.canClaim();
  canConclude = (item: AdjudicationCase): boolean => this.canAdjudicate() && item.reviewed_by === this.auth.user()?.id;
  ngOnInit(): void { this.reload(); }
  reload(): void { this.datasets.load(); this.schemas.load(); this.annotations.load(); this.cases.load(); }
  choose(item: AdjudicationCase): void { this.cases.choose(item); this.localError.set(''); this.useLeftLabels(); }
  compute(): void {
    const ids = this.computeAnnotationIds.split(',').map((value) => Number(value.trim())).filter((value) => Number.isInteger(value) && value > 0);
    if (!this.computeDatasetId || this.computeItemKey.trim().length < 2 || ids.length < 2) { this.localError.set('Choose a dataset, item key, and at least two annotation IDs.'); return; }
    this.localError.set(''); this.cases.compute({ dataset_id: this.computeDatasetId, item_key: this.computeItemKey.trim(), annotation_set_ids: ids, metric: this.computeMetric });
  }
  useLeftLabels(): void { this.finalLabelsText = JSON.stringify(this.selectedAnnotations()[0]?.labels ?? [], null, 2); }
  decide(item: AdjudicationCase): void {
    let labels: AnnotationLabel[];
    try { labels = JSON.parse(this.finalLabelsText) as AnnotationLabel[]; } catch { this.localError.set('Final labels must be a valid JSON array.'); return; }
    if (!Array.isArray(labels) || labels.length === 0 || this.rationale.trim().length < 12) { this.localError.set('Provide at least one final label and a rationale of 12 or more characters.'); return; }
    this.localError.set(''); this.cases.decide(item, labels, this.rationale.trim());
  }
  metricName(metric: string): string { return metric === 'cohen_kappa' ? "Cohen's Kappa" : "Krippendorff's Alpha"; }
  interval(label: AnnotationLabel): string { return label.end !== undefined && label.start !== undefined && label.end > label.start ? `[${label.start}, ${label.end})` : 'class'; }
}
