package svgcheck_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/justsml/pre-print-check/svgcheck"
)

func TestProjectReportBuildsPortableContract(t *testing.T) {
	report, err := svgcheck.Check([]byte(`<svg width="100" height="50"><script>alert(1)</script><rect onclick="x()"/></svg>`), "vinyl")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	portable := svgcheck.ProjectReport(report)
	if portable.Target != "vinyl" || !strings.Contains(portable.TargetDetails, "vinyl/decal output") {
		t.Fatalf("unexpected target projection: %#v", portable)
	}
	if portable.Counts.Errors != 2 {
		t.Fatalf("expected two errors, got %#v", portable.Counts)
	}

	var script *svgcheck.PortableIssue
	for i := range portable.Issues {
		if portable.Issues[i].Code == "script" {
			script = &portable.Issues[i]
			break
		}
	}
	if script == nil {
		t.Fatal("expected projected script finding")
	}
	if script.FixCategory != "safety" || !script.UnsafeRequired || !script.AutomaticFix {
		t.Fatalf("unexpected projected remediation: %#v", script)
	}
	if len(portable.FixCategories) == 0 || portable.FixCategories[0] != "metadata" {
		t.Fatalf("unexpected automatic fix categories: %#v", portable.FixCategories)
	}
}

func TestPortableReportJSONContract(t *testing.T) {
	report, err := svgcheck.Check([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M 0 0 L 1 1" stroke="black" stroke-width="0.2"/></svg>`), "paper@10in")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	body, err := json.Marshal(svgcheck.ProjectReport(report))
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	for _, field := range []string{`"friendlySummary"`, `"counts"`, `"meta"`, `"issues"`, `"fixCategories"`, `"smallShapesSub1mm"`} {
		if !strings.Contains(string(body), field) {
			t.Fatalf("portable JSON is missing %s: %s", field, body)
		}
	}
}
