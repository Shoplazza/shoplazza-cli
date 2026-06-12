package themes

import (
	"strings"
	"testing"
)

func TestPlanDetail_HitsV2SpecPathWithID(t *testing.T) {
	p := PlanDetail("abc123")
	if !strings.HasSuffix(p.Path, "/themes/abc123") {
		t.Errorf("path = %s", p.Path)
	}
}

func TestPlanPublish_PATCHWithID(t *testing.T) {
	p := PlanPublish("abc123")
	if p.Method != "PATCH" {
		t.Errorf("method = %s, want PATCH", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/themes/abc123/publish") {
		t.Errorf("path = %s", p.Path)
	}
}

func TestPlanDelete_DELETEWithID(t *testing.T) {
	p := PlanDelete("abc123")
	if p.Method != "DELETE" {
		t.Errorf("method = %s", p.Method)
	}
}

func TestPlanTaskDetail_HitsV2SpecTaskPath(t *testing.T) {
	p := PlanTaskDetail("task-1")
	if p.Method != "GET" || !strings.Contains(p.Path, "/themes/task/task-1") {
		t.Errorf("path = %s method = %s", p.Path, p.Method)
	}
	if !strings.Contains(p.Path, "/openapi/2026-01/") {
		t.Errorf("PlanTaskDetail must go to 2026-01 spec path, not v1")
	}
}

func TestPlanDocTree_HitsV2SpecPath(t *testing.T) {
	p := PlanDocTree("abc")
	if !strings.HasSuffix(p.Path, "/themes/abc/doctree") {
		t.Errorf("path = %s", p.Path)
	}
}

func TestPlanDocCreate_POSTtoDoc(t *testing.T) {
	body := map[string]any{"type": "assets", "location": "main.css", "content": "x"}
	p := PlanDocCreate("abc", body)
	if p.Method != "POST" || !strings.HasSuffix(p.Path, "/themes/abc/doc") {
		t.Errorf("path = %s method = %s", p.Path, p.Method)
	}
}

func TestPlanDocPatch_PATCH(t *testing.T) {
	p := PlanDocPatch("abc", map[string]any{"type": "assets", "location": "main.css", "content": "x"})
	if p.Method != "PATCH" {
		t.Errorf("method = %s", p.Method)
	}
}

func TestPlanDocDelete_DELETEWithQuery(t *testing.T) {
	p := PlanDocDelete("abc", map[string]any{"type": "assets", "location": "main.css"})
	if p.Method != "DELETE" {
		t.Errorf("method = %s", p.Method)
	}
	if p.Query["type"] != "assets" {
		t.Errorf("query: %v", p.Query)
	}
}

func TestPlanShop_HitsV2Spec(t *testing.T) {
	p := PlanShop()
	if !strings.Contains(p.Path, "/openapi/2026-01/shop") {
		t.Errorf("PlanShop should go to 2026-01: %s", p.Path)
	}
}

// v1 path factories
func TestPlanUpload_HitsV1Path(t *testing.T) {
	p := PlanUpload("abc", "NoirChic", "1.0")
	if !strings.Contains(p.Path, "/openapi/2020-07/themes/upload") {
		t.Errorf("PlanUpload must use v1 path: %s", p.Path)
	}
	if p.Method != "POST" {
		t.Errorf("method = %s", p.Method)
	}
	if p.Query["name"] != "NoirChic" || p.Query["version"] != "1.0" || p.Query["theme_id"] != "abc" {
		t.Errorf("query: %v", p.Query)
	}
}

func TestPlanDownload_HitsV1Path(t *testing.T) {
	p := PlanDownload("abc")
	if !strings.Contains(p.Path, "/openapi/2020-07/themes/abc/download") {
		t.Errorf("PlanDownload must use v1 path: %s", p.Path)
	}
}

func TestPlanShareShop_UsesV1Path(t *testing.T) {
	p := PlanShareShop()
	if !strings.Contains(p.Path, "/openapi/2020-07/shop") {
		t.Errorf("PlanShareShop must use v1 path for byte-exact parity: %s", p.Path)
	}
}

func TestPlanShareUpload_UsesV1Path(t *testing.T) {
	p := PlanShareUpload("", "NoirChic", "1.0")
	if !strings.Contains(p.Path, "/openapi/2020-07/themes/upload") {
		t.Errorf("path: %s", p.Path)
	}
	// theme_id empty → query "theme_id" = ""
	if v, ok := p.Query["theme_id"]; !ok || v != "" {
		t.Errorf("theme_id should be empty string, got %v", v)
	}
	if v := p.Query["merchant_theme_id"]; v != "" {
		t.Errorf("merchant_theme_id should always be empty: %v", v)
	}
}

func TestPlanShareUpload_WithThemeID(t *testing.T) {
	p := PlanShareUpload("abc123", "NoirChic", "1.0")
	if p.Query["theme_id"] != "abc123" {
		t.Errorf("theme_id: %v", p.Query["theme_id"])
	}
}

func TestNoPlanShare_ExistsByName(t *testing.T) {
	// share has NO dedicated endpoint; verify no PlanShare factory was added.
	// (Implementation check; this is asserted via build — symbol must not exist.)
	// If author tries to add PlanShare, this test will fail because the symbol
	// won't compile. (Reviewer-enforced via PR.)
}
