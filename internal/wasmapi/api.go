//go:build js && wasm

package wasmapi

import (
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strings"
	"syscall/js"

	"github.com/justsml/pre-print-check/svgcheck"
)

type apiResponse struct {
	OK            bool                     `json:"ok"`
	Error         string                   `json:"error,omitempty"`
	Report        *svgcheck.PortableReport `json:"report,omitempty"`
	Overlay       string                   `json:"overlay,omitempty"`
	SVG           string                   `json:"svg,omitempty"`
	Changes       []string                 `json:"changes,omitempty"`
	Skipped       []string                 `json:"skipped,omitempty"`
	FixCategories []string                 `json:"fixCategories,omitempty"`
}

var registeredFuncs []js.Func

func ServeFull(globalName string) {
	api := js.Global().Get("Object").New()
	register(api, "check", check)
	register(api, "overlay", overlay)
	register(api, "fix", fix)
	register(api, "fixCategories", fixCategories)
	serve(globalName, api)
}

func ServeCheck(globalName string) {
	api := js.Global().Get("Object").New()
	register(api, "check", check)
	serve(globalName, api)
}

func ServeFix(globalName string) {
	api := js.Global().Get("Object").New()
	register(api, "fix", fix)
	register(api, "fixCategories", fixCategories)
	serve(globalName, api)
}

func serve(globalName string, api js.Value) {
	api.Set("ready", true)
	js.Global().Set(globalName, api)
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
	portable := svgcheck.ProjectReport(report)
	return apiResponse{OK: true, Report: &portable}
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

	return apiResponse{
		OK:      true,
		SVG:     string(result.SVG),
		Changes: result.Changes,
		Skipped: result.Skipped,
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
