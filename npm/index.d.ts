export interface PrePrintCheckOptions {
  target?: string;
}

export interface LoadOptions {
  wasmURL?: string | URL;
  wasmExecURL?: string | URL;
  timeoutMs?: number;
}

export interface FixOptions extends PrePrintCheckOptions {
  categories?: string | string[];
  fix?: string | string[];
  unsafe?: boolean;
}

export interface PrePrintCheckCounts {
  errors: number;
  warnings: number;
  info: number;
}

export interface PrePrintCheckMeta {
  width?: string;
  height?: string;
  viewBox?: string;
  rasterImages: number;
  inlineRasterImages: number;
  textElements: number;
  filters: number;
  filterRefs: number;
  shadows: number;
  uniqueColors: number;
  thinStrokes: number;
  nearDisconnected: number;
  smallShapesSub1mm: number;
  smallShapesSub2mm: number;
  missingBleedShapes: number;
  safeAreaRiskShapes: number;
  backgroundTransparency: number;
}

export interface PrePrintCheckIssue {
  severity: "error" | "warning" | "info" | string;
  code: string;
  message: string;
  rank?: "low" | "moderate" | "high" | string;
  fixCategory?: string;
  unsafeRequired?: boolean;
  automaticFix: boolean;
}

export interface PrePrintCheckReport {
  summary: string;
  friendlySummary: string;
  target?: string;
  targetDetails?: string;
  counts: PrePrintCheckCounts;
  meta: PrePrintCheckMeta;
  issues: PrePrintCheckIssue[];
  fixCategories: string[];
}

export interface FixResult {
  svg: string;
  changes: string[];
  skipped: string[];
  report: PrePrintCheckReport;
  overlay: string;
}

export interface PrePrintCheckAPI {
  check(svg: string, options?: PrePrintCheckOptions): PrePrintCheckReport;
  overlay(svg: string, options?: PrePrintCheckOptions): string;
  fix(svg: string, options?: FixOptions): FixResult;
  fixCategories(): string[];
}

export function loadPrePrintCheck(options?: LoadOptions): Promise<PrePrintCheckAPI>;
export function check(svg: string, options?: PrePrintCheckOptions): Promise<PrePrintCheckReport>;
export function overlay(svg: string, options?: PrePrintCheckOptions): Promise<string>;
export function fix(svg: string, options?: FixOptions): Promise<FixResult>;
export function fixCategories(): Promise<string[]>;
