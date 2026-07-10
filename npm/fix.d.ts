export type { FixOptions, FixResult, LoadOptions } from "./index.js";

import type { FixOptions, FixResult, LoadOptions } from "./index.js";

export interface PrePrintCheckFixAPI {
  fix(svg: string, options?: FixOptions): FixResult;
  fixCategories(): string[];
}

export function loadPrePrintCheck(options?: LoadOptions): Promise<PrePrintCheckFixAPI>;
export function fix(svg: string, options?: FixOptions): Promise<FixResult>;
export function fixCategories(): Promise<string[]>;
