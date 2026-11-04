package kagi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestCollectionsReturnsNonEmptyInDocumentedOrder(t *testing.T) {
	d := &SearchData{
		RelatedSearch:    []Result{{Title: "r"}},
		Search:           []Result{{Title: "s1"}, {Title: "s2"}},
		Infobox:          []Result{{Title: "i"}},
		DirectAnswer:     []Result{{Title: "d"}},
		AdjacentQuestion: []Result{},
		News:             nil,
	}

	got := d.Collections()
	wantNames := []string{"direct_answer", "infobox", "search", "related_search"}
	wantLabels := []string{"Direct Answer", "Infobox", "Results", "Related Searches"}
	wantLens := []int{1, 1, 2, 1}

	if len(got) != len(wantNames) {
		t.Fatalf("len(Collections()) = %d, want %d: %+v", len(got), len(wantNames), got)
	}
	for i, c := range got {
		if c.Name != wantNames[i] {
			t.Errorf("collection[%d].Name = %q, want %q", i, c.Name, wantNames[i])
		}
		if c.Label != wantLabels[i] {
			t.Errorf("collection[%d].Label = %q, want %q", i, c.Label, wantLabels[i])
		}
		if len(c.Results) != wantLens[i] {
			t.Errorf("collection[%d] (%s) has %d results, want %d", i, c.Name, len(c.Results), wantLens[i])
		}
	}
}

func TestCollectionsEmpty(t *testing.T) {
	var d SearchData
	if got := d.Collections(); len(got) != 0 {
		t.Errorf("Collections() on zero SearchData = %+v, want empty", got)
	}
}

func TestTotalSumsAcrossCollections(t *testing.T) {
	d := &SearchData{
		Search:        []Result{{}, {}, {}},
		Image:         []Result{{}},
		RelatedSearch: []Result{{}, {}},
	}
	if got := d.Total(); got != 6 {
		t.Errorf("Total() = %d, want 6", got)
	}
	if got := (&SearchData{}).Total(); got != 0 {
		t.Errorf("Total() on empty = %d, want 0", got)
	}
}

func TestQuestionReadsStringProp(t *testing.T) {
	r := Result{Props: map[string]any{"question": "Who founded Apple?"}}
	if got := r.Question(); got != "Who founded Apple?" {
		t.Errorf("Question() = %q", got)
	}
}

func TestQuestionEmptyWhenAbsentOrNonString(t *testing.T) {
	cases := map[string]Result{
		"nil props":    {},
		"missing key":  {Props: map[string]any{"language": "en"}},
		"non-string":   {Props: map[string]any{"question": 42}},
		"nil value":    {Props: map[string]any{"question": nil}},
		"string array": {Props: map[string]any{"question": []string{"a"}}},
	}
	for name, r := range cases {
		t.Run(name, func(t *testing.T) {
			if got := r.Question(); got != "" {
				t.Errorf("Question() = %q, want empty", got)
			}
		})
	}
}

func TestLensIsZero(t *testing.T) {
	var nilLens *Lens
	if !nilLens.IsZero() {
		t.Error("nil *Lens IsZero() = false, want true")
	}
	if !(&Lens{}).IsZero() {
		t.Error("empty Lens IsZero() = false, want true")
	}

	setters := map[string]func(*Lens){
		"SitesIncluded":    func(l *Lens) { l.SitesIncluded = []string{"a.com"} },
		"SitesExcluded":    func(l *Lens) { l.SitesExcluded = []string{"a.com"} },
		"KeywordsIncluded": func(l *Lens) { l.KeywordsIncluded = []string{"k"} },
		"KeywordsExcluded": func(l *Lens) { l.KeywordsExcluded = []string{"k"} },
		"FileType":         func(l *Lens) { l.FileType = "pdf" },
		"TimeAfter":        func(l *Lens) { l.TimeAfter = "2026-01-01" },
		"TimeBefore":       func(l *Lens) { l.TimeBefore = "2026-01-01" },
		"TimeRelative":     func(l *Lens) { l.TimeRelative = "day" },
		"SearchRegion":     func(l *Lens) { l.SearchRegion = "us" },
	}
	for name, set := range setters {
		t.Run(name, func(t *testing.T) {
			l := &Lens{}
			set(l)
			if l.IsZero() {
				t.Errorf("IsZero() = true with %s set, want false", name)
			}
		})
	}
}

func TestFiltersIsZero(t *testing.T) {
	var nilFilters *Filters
	if !nilFilters.IsZero() {
		t.Error("nil *Filters IsZero() = false, want true")
	}
	if !(&Filters{}).IsZero() {
		t.Error("empty Filters IsZero() = false, want true")
	}

	f := false
	setters := map[string]*Filters{
		"Region":     {Region: "us"},
		"After":      {After: "2026-01-01"},
		"Before":     {Before: "2026-01-01"},
		"SafeSearch": {SafeSearch: &f}, // a pointer to false still counts as set
	}
	for name, filters := range setters {
		if filters.IsZero() {
			t.Errorf("IsZero() = true with %s set, want false", name)
		}
	}
}

func TestSearchEmptyQueryMakesNoRequest(t *testing.T) {
	srv, count := newStub(t, writeJSON(http.StatusOK, okSearchBody))
	c := newTestClient(srv.URL, 0)

	if _, err := c.Search(context.Background(), SearchRequest{}); err == nil {
		t.Error("Search with empty query returned nil error")
	}
	if _, err := c.SearchMarkdown(context.Background(), SearchRequest{}); err == nil {
		t.Error("SearchMarkdown with empty query returned nil error")
	}
	if count.Load() != 0 {
		t.Errorf("request count = %d, want 0", count.Load())
	}
}

func TestSearchRequestOmitsUnsetOptionalFields(t *testing.T) {
	b, err := json.Marshal(SearchRequest{Query: "rust"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := m["query"]; !ok {
		t.Error(`"query" key missing`)
	}
	for _, key := range []string{"workflow", "limit", "page", "filters", "lens", "extract", "timeout", "format", "lens_id"} {
		if _, ok := m[key]; ok {
			t.Errorf("key %q emitted for zero value; want omitted (got %s)", key, b)
		}
	}
	if len(m) != 1 {
		t.Errorf("emitted keys = %s, want only query", b)
	}
}

func TestSearchRequestEmitsSetFields(t *testing.T) {
	b, err := json.Marshal(SearchRequest{
		Query:    "rust",
		Workflow: WorkflowNews,
		Limit:    5,
		Page:     2,
		Filters:  &Filters{Region: "us"},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"workflow", "limit", "page", "filters"} {
		if _, ok := m[key]; !ok {
			t.Errorf("key %q missing from %s", key, b)
		}
	}
}
