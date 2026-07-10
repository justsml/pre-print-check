export type {
  LoadOptions,
  PrePrintCheckCounts,
  PrePrintCheckIssue,
  PrePrintCheckMeta,
  PrePrintCheckOptions,
  PrePrintCheckReport,
} from "./index.js";

import type { LoadOptions, PrePrintCheckOptions, PrePrintCheckReport } from "./index.js";

export interface PrePrintCheckAPI {
  check(svg: string, options?: PrePrintCheckOptions): PrePrintCheckReport;
}

export function loadPrePrintCheck(options?: LoadOptions): Promise<PrePrintCheckAPI>;
export function check(svg: string, options?: PrePrintCheckOptions): Promise<PrePrintCheckReport>;
