package svgcheck

type findingAdvisory struct {
	Category FixCategory
	Message  string
}

type findingPolicy struct {
	FixCategory    FixCategory
	UnsafeRequired bool
	AutomaticFix   bool
	Advisories     []findingAdvisory
}

func automaticPolicy(category FixCategory, unsafeRequired bool, advisories ...findingAdvisory) findingPolicy {
	return findingPolicy{
		FixCategory:    category,
		UnsafeRequired: unsafeRequired,
		AutomaticFix:   true,
		Advisories:     advisories,
	}
}

func manualPolicy(category FixCategory, message string) findingPolicy {
	return findingPolicy{
		FixCategory: category,
		Advisories:  []findingAdvisory{{Category: category, Message: message}},
	}
}

var findingPolicies = map[string]findingPolicy{
	"missing-xmlns":   automaticPolicy(FixCategoryMetadata, false),
	"missing-viewbox": automaticPolicy(FixCategoryMetadata, false),
	"missing-size":    automaticPolicy(FixCategoryMetadata, false),
	"missing-bleed":   automaticPolicy(FixCategoryBleed, false),
	"script":          automaticPolicy(FixCategorySafety, true),
	"event-handler":   automaticPolicy(FixCategorySafety, true),

	"shadow-effect":                    automaticPolicy(FixCategoryEffects, true),
	"print-effects-require-flattening": automaticPolicy(FixCategoryEffects, true),
	"fabric-effects":                   automaticPolicy(FixCategoryEffects, true),
	"large-format-effects":             automaticPolicy(FixCategoryEffects, true),
	"effects-may-not-output": automaticPolicy(FixCategoryEffects, true, findingAdvisory{
		Category: FixCategoryCutter,
		Message:  "cutter fixes may need manual path reconstruction; use --unsafe --fix raster,effects only to strip incompatible content",
	}),

	"raster-image":        automaticPolicy(FixCategoryRaster, true),
	"inline-raster-image": automaticPolicy(FixCategoryRaster, true),
	"large-format-raster": automaticPolicy(FixCategoryRaster, true),
	"raster-not-cuttable": automaticPolicy(FixCategoryRaster, true, findingAdvisory{
		Category: FixCategoryCutter,
		Message:  "cutter fixes may need manual path reconstruction; use --unsafe --fix raster,effects only to strip incompatible content",
	}),

	"external-reference":           manualPolicy(FixCategoryReferences, "external references need manual packaging or embedding"),
	"packaging-external-reference": manualPolicy(FixCategoryReferences, "external references need manual packaging or embedding"),

	"color-count":          manualPolicy(FixCategoryColors, "color fixes need manual CMYK/spot-color conversion and proofing"),
	"rgb-colors-for-print": manualPolicy(FixCategoryColors, "color fixes need manual CMYK/spot-color conversion and proofing"),
	"cmyk-in-svg":          manualPolicy(FixCategoryColors, "color fixes need manual CMYK/spot-color conversion and proofing"),
	"many-fabric-colors":   manualPolicy(FixCategoryColors, "color fixes need manual CMYK/spot-color conversion and proofing"),

	"thin-stroke":             manualPolicy(FixCategoryStrokes, "thin strokes need manual review before thickening or outlining"),
	"near-disconnected-lines": manualPolicy(FixCategoryGeometry, "near-disconnected line joins need manual node joining or shape rebuilding"),
	"text-not-outlined":       manualPolicy(FixCategoryTypography, "text fixes need manual outlining, knockouts, or layout changes"),
	"text-overlap-shapes":     manualPolicy(FixCategoryTypography, "text fixes need manual outlining, knockouts, or layout changes"),
	"small-detail-durability": manualPolicy(FixCategoryDetail, "small-detail durability fixes need manual simplification or production-method changes"),

	"low-effective-ppi":       manualPolicy(FixCategorySizing, "sizing/resolution fixes need a chosen physical output size or higher-resolution source art"),
	"modest-effective-ppi":    manualPolicy(FixCategorySizing, "sizing/resolution fixes need a chosen physical output size or higher-resolution source art"),
	"oversized-for-target":    manualPolicy(FixCategorySizing, "sizing/resolution fixes need a chosen physical output size or higher-resolution source art"),
	"target-size-recommended": manualPolicy(FixCategorySizing, "sizing/resolution fixes need a chosen physical output size or higher-resolution source art"),

	"safe-area-risk":          manualPolicy(FixCategoryBleed, "safe-area fixes need manual layout movement inward from trim"),
	"background-transparency": manualPolicy(FixCategoryEffects, "background transparency fixes need the intended substrate/background color"),
}

func findingPolicyForCode(code string) findingPolicy {
	return findingPolicies[code]
}

func reportHasAutomaticFix(report Report, category FixCategory) bool {
	for _, issue := range report.Issues {
		if issue.AutomaticFix && issue.FixCategory == category {
			return true
		}
	}
	return false
}

func advisoryFixNotes(report Report, categories map[FixCategory]bool) []string {
	var notes []string
	for _, issue := range report.Issues {
		policy := findingPolicyForCode(issue.Code)
		for _, advisory := range policy.Advisories {
			if categories[advisory.Category] {
				notes = append(notes, advisory.Message)
			}
		}
	}
	return dedupeStrings(notes)
}
