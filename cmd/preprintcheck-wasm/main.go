//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strings"
	"syscall/js"

	"github.com/justsml/pre-print-check/internal/svgcheck"
)

type apiResponse struct {
	OK            bool       `json:"ok"`
	Error         string     `json:"error,omitempty"`
	Report        *apiReport `json:"report,omitempty"`
	Overlay       string     `json:"overlay,omitempty"`
	SVG           string     `json:"svg,omitempty"`
	Changes       []string   `json:"changes,omitempty"`
	Skipped       []string   `json:"skipped,omitempty"`
	FixCategories []string   `json:"fixCategories,omitempty"`
}

type apiReport struct {
	Summary         string     `json:"summary"`
	FriendlySummary string     `json:"friendlySummary"`
	Target          string     `json:"target,omitempty"`
	TargetDetails   string     `json:"targetDetails,omitempty"`
	Counts          apiCounts  `json:"counts"`
	Meta            apiMeta    `json:"meta"`
	Issues          []apiIssue `json:"issues"`
	FixCategories   []string   `json:"fixCategories"`
}

type apiCounts struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
}

type apiMeta struct {
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

type apiIssue struct {
	Severity       string `json:"severity"`
	Code           string `json:"code"`
	Message        string `json:"message"`
	Rank           string `json:"rank,omitempty"`
	FixCategory    string `json:"fixCategory,omitempty"`
	UnsafeRequired bool   `json:"unsafeRequired,omitempty"`
	AutomaticFix   bool   `json:"automaticFix"`
}

var registeredFuncs []js.Func

func main() {
	api := js.Global().Get("Object").New()
	register(api, "check", check)
	register(api, "overlay", overlay)
	register(api, "fix", fix)
	register(api, "fixCategories", fixCategories)
	api.Set("ready", true)
	js.Global().Set("prePrintCheck", api)
	select {}
}

func register(api js.Value, name string, fn func([]js.Value) apiResponse) {
	wrapped := js.FuncOf(func(this js.Value, args []js.Value) any {
		return encodeResponse(func() apiResponse {
			return fn(args)
		})
	})
	registeredFuncs = append(registeredFuncs, wrapped)
	api.Set(name, wrapped)
}

func encodeResponse(fn func() apiResponse) (out string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			body, _ := json.Marshal(apiResponse{
				OK:    false,
				Error: fmt.Sprintf("pre-print-check WASM panic: %v\n%s", recovered, debug.Stack()),
			})
			out = string(body)
		}
	}()
	body, err := json.Marshal(fn())
	if err != nil {
		body, _ = json.Marshal(apiResponse{OK: false, Error: err.Error()})
	}
	return string(body)
}

func check(args []js.Value) apiResponse {
	input, opts := parseCall(args)
	report, err := svgcheck.Check([]byte(input), opts.target)
	if err != nil {
		return apiResponse{OK: false, Error: err.Error()}
	}
	return apiResponse{OK: true, Report: webReport(report)}
}

func overlay(args []js.Value) apiResponse {
	input, opts := parseCall(args)
	out, err := svgcheck.GenerateOverlay([]byte(input), svgcheck.OverlayOptions{Target: opts.target})
	if err != nil {
		return apiResponse{OK: false, Error: err.Error()}
	}
	return apiResponse{OK: true, Overlay: string(out)}
}

func fix(args []js.Value) apiResponse {
	input, opts := parseCall(args)
	result, err := svgcheck.Fix([]byte(input), svgcheck.FixOptions{
		Target:     opts.target,
		Unsafe:     opts.unsafe,
		Categories: opts.categories,
	})
	if err != nil {
		return apiResponse{OK: false, Error: err.Error()}
	}

	report, err := svgcheck.Check(result.SVG, opts.target)
	if err != nil {
		return apiResponse{OK: false, Error: err.Error()}
	}
	overlaySVG, err := svgcheck.GenerateOverlay(result.SVG, svgcheck.OverlayOptions{Target: opts.target})
	if err != nil {
		return apiResponse{OK: false, Error: err.Error()}
	}

	return apiResponse{
		OK:      true,
		SVG:     string(result.SVG),
		Changes: result.Changes,
		Skipped: result.Skipped,
		Report:  webReport(report),
		Overlay: string(overlaySVG),
	}
}

func fixCategories(args []js.Value) apiResponse {
	return apiResponse{OK: true, FixCategories: svgcheck.FixCategoryNames()}
}

type callOptions struct {
	target     string
	categories []string
	unsafe     bool
}

func parseCall(args []js.Value) (string, callOptions) {
	var input string
	if len(args) > 0 && args[0].Type() == js.TypeString {
		input = args[0].String()
	}

	opts := callOptions{}
	if len(args) < 2 || args[1].IsUndefined() || args[1].IsNull() {
		return input, opts
	}

	raw := args[1]
	opts.target = stringProp(raw, "target")
	opts.unsafe = boolProp(raw, "unsafe")
	opts.categories = stringArrayProp(raw, "categories")
	return input, opts
}

func stringProp(value js.Value, name string) string {
	prop := value.Get(name)
	if prop.IsUndefined() || prop.IsNull() {
		return ""
	}
	return prop.String()
}

func boolProp(value js.Value, name string) bool {
	prop := value.Get(name)
	if prop.IsUndefined() || prop.IsNull() {
		return false
	}
	return prop.Bool()
}

func stringArrayProp(value js.Value, name string) []string {
	prop := value.Get(name)
	if prop.IsUndefined() || prop.IsNull() {
		return nil
	}
	if prop.Type() == js.TypeString {
		return []string{prop.String()}
	}
	length := prop.Get("length")
	if length.IsUndefined() || length.IsNull() {
		return nil
	}
	out := make([]string, 0, length.Int())
	for i := 0; i < length.Int(); i++ {
		item := strings.TrimSpace(prop.Index(i).String())
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func webReport(report svgcheck.Report) *apiReport {
	errors, warnings, info := report.IssueCounts()
	issues := make([]apiIssue, 0, len(report.Issues))
	fixSeen := map[string]bool{}
	var fixCategories []string

	for _, issue := range report.Issues {
		category, unsafeRequired, automaticFix := issueFixAction(issue.Code)
		if automaticFix && !fixSeen[category] {
			fixSeen[category] = true
			fixCategories = append(fixCategories, category)
		}
		issues = append(issues, apiIssue{
			Severity:       string(issue.Severity),
			Code:           issue.Code,
			Message:        issue.Message,
			Rank:           string(issue.Rank),
			FixCategory:    category,
			UnsafeRequired: unsafeRequired,
			AutomaticFix:   automaticFix,
		})
	}

	targetDetails := ""
	if report.Target.Raw != "" {
		targetDetails = report.Target.Description()
	}

	return &apiReport{
		Summary:         report.Summary(),
		FriendlySummary: report.FriendlySummary(),
		Target:          report.Target.Raw,
		TargetDetails:   targetDetails,
		Counts:          apiCounts{Errors: errors, Warnings: warnings, Info: info},
		Meta: apiMeta{
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

func issueFixAction(code string) (category string, unsafeRequired bool, automaticFix bool) {
	switch code {
	case "missing-xmlns", "missing-viewbox", "missing-size":
		return string(svgcheck.FixCategoryMetadata), false, true
	case "missing-bleed":
		return string(svgcheck.FixCategoryBleed), false, true
	case "script", "event-handler":
		return string(svgcheck.FixCategorySafety), true, true
	case "shadow-effect", "effects-may-not-output", "print-effects-require-flattening", "fabric-effects", "large-format-effects":
		return string(svgcheck.FixCategoryEffects), true, true
	case "raster-image", "inline-raster-image", "raster-not-cuttable", "large-format-raster":
		return string(svgcheck.FixCategoryRaster), true, true
	default:
		return "", false, false
	}
}
