export interface PrePrintOptions {
  target?: string;
}

export interface LoadOptions {
  wasmURL?: string | URL;
  wasmExecURL?: string | URL;
  timeoutMs?: number;
}

export interface FixOptions extends PrePrintOptions {
  categories?: string | string[];
  fix?: string | string[];
  unsafe?: boolean;
}

export interface PrePrintCounts {
  errors: number;
  warnings: number;
  info: number;
}

export interface PrePrintMeta {
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

export interface PrePrintIssue {
  severity: "error" | "warning" | "info" | string;
  code: string;
  message: string;
  rank?: "low" | "moderate" | "high" | string;
  fixCategory?: string;
  unsafeRequired?: boolean;
  automaticFix: boolean;
}

export interface PrePrintReport {
  summary: string;
  friendlySummary: string;
  target?: string;
  targetDetails?: string;
  counts: PrePrintCounts;
  meta: PrePrintMeta;
  issues: PrePrintIssue[];
  fixCategories: string[];
}

export interface FixResult {
  svg: string;
  changes: string[];
  skipped: string[];
  report: PrePrintReport;
  overlay: string;
}

export interface PrePrintAPI {
  check(svg: string, options?: PrePrintOptions): PrePrintReport;
  overlay(svg: string, options?: PrePrintOptions): string;
  fix(svg: string, options?: FixOptions): FixResult;
  fixCategories(): string[];
}

export function loadPrePrint(options?: LoadOptions): Promise<PrePrintAPI>;
export function check(svg: string, options?: PrePrintOptions): Promise<PrePrintReport>;
export function overlay(svg: string, options?: PrePrintOptions): Promise<string>;
export function fix(svg: string, options?: FixOptions): Promise<FixResult>;
export function fixCategories(): Promise<string[]>;
