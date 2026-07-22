package svgcheck

type PortableReport struct {
	Summary         string          `json:"summary"`
	FriendlySummary string          `json:"friendlySummary"`
	Target          string          `json:"target,omitempty"`
	TargetDetails   string          `json:"targetDetails,omitempty"`
	Counts          PortableCounts  `json:"counts"`
	Meta            PortableMeta    `json:"meta"`
	Issues          []PortableIssue `json:"issues"`
	FixCategories   []string        `json:"fixCategories"`
}

type PortableCounts struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
}

type PortableMeta struct {
	Width                  string `json:"width,omitempty"`
	Height                 string `json:"height,omitempty"`
	ViewBox                string `json:"viewBox,omitempty"`
	RasterImages           int    `json:"rasterImages"`
	InlineRasterImages     int    `json:"inlineRasterImages"`
	TextElements           int    `json:"textElements"`
	Filters                int    `json:"filters"`
	FilterRefs             int    `json:"filterRefs"`
	Shadows                int    `json:"shadows"`
	UniqueColors           int    `json:"uniqueColors"`
	ThinStrokes            int    `json:"thinStrokes"`
	NearDisconnected       int    `json:"nearDisconnected"`
	SmallShapesSub1MM      int    `json:"smallShapesSub1mm"`
	SmallShapesSub2MM      int    `json:"smallShapesSub2mm"`
	MissingBleedShapes     int    `json:"missingBleedShapes"`
	SafeAreaRiskShapes     int    `json:"safeAreaRiskShapes"`
	BackgroundTransparency int    `json:"backgroundTransparency"`
}

type PortableIssue struct {
	Severity       string `json:"severity"`
	Code           string `json:"code"`
	Message        string `json:"message"`
	Rank           string `json:"rank,omitempty"`
	FixCategory    string `json:"fixCategory,omitempty"`
	UnsafeRequired bool   `json:"unsafeRequired,omitempty"`
	AutomaticFix   bool   `json:"automaticFix"`
}

func ProjectReport(report Report) PortableReport {
	errors, warnings, info := report.IssueCounts()
	issues := make([]PortableIssue, 0, len(report.Issues))
	fixSeen := map[string]bool{}
	var fixCategories []string

	for _, issue := range report.Issues {
		category := string(issue.FixCategory)
		if issue.AutomaticFix && !fixSeen[category] {
			fixSeen[category] = true
			fixCategories = append(fixCategories, category)
		}
		issues = append(issues, PortableIssue{
			Severity:       string(issue.Severity),
			Code:           issue.Code,
			Message:        issue.Message,
			Rank:           string(issue.Rank),
			FixCategory:    category,
			UnsafeRequired: issue.UnsafeRequired,
			AutomaticFix:   issue.AutomaticFix,
		})
	}

	targetDetails := ""
	if report.Target.Raw != "" {
		targetDetails = report.Target.Description()
	}

	return PortableReport{
		Summary:         report.Summary(),
		FriendlySummary: report.FriendlySummary(),
		Target:          report.Target.Raw,
		TargetDetails:   targetDetails,
		Counts:          PortableCounts{Errors: errors, Warnings: warnings, Info: info},
		Meta: PortableMeta{
			Width:                  report.Meta.Width,
			Height:                 report.Meta.Height,
			ViewBox:                report.Meta.ViewBox,
			RasterImages:           report.Meta.RasterImages,
			InlineRasterImages:     report.Meta.InlineRasterImages,
			TextElements:           report.Meta.TextElements,
			Filters:                report.Meta.Filters,
			FilterRefs:             report.Meta.FilterRefs,
			Shadows:                report.Meta.Shadows,
			UniqueColors:           report.Meta.UniqueColors,
			ThinStrokes:            report.Meta.ThinStrokes,
			NearDisconnected:       report.Meta.NearDisconnected,
			SmallShapesSub1MM:      report.Meta.SmallShapesSub1MM,
			SmallShapesSub2MM:      report.Meta.SmallShapesSub2MM,
			MissingBleedShapes:     report.Meta.MissingBleedShapes,
			SafeAreaRiskShapes:     report.Meta.SafeAreaRiskShapes,
			BackgroundTransparency: report.Meta.BackgroundTransparency,
		},
		Issues:        issues,
		FixCategories: fixCategories,
	}
}
