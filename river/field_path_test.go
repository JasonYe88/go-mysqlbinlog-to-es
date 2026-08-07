package river

import (
	"reflect"
	"testing"
)

func TestParseFieldMapping(t *testing.T) {
	cases := []struct {
		k, v                   string
		col, path, elastic, typ string
	}{
		{"title", "es_title", "title", "", "es_title", ""},
		{"tags", "es_tags,list", "tags", "", "es_tags", "list"},
		{"keywords", ",list", "keywords", "", "keywords", "list"},
		{"ext.sku", "sku", "ext", "sku", "sku", ""},
		{"ext.sku", "data.sku", "ext", "sku", "data.sku", ""},
		{"ext.info.name", "data.name", "ext", "info.name", "data.name", ""},
		{"ext.tags", "tags,list", "ext", "tags", "tags", "list"},
		{"ext.sku", ",list", "ext", "sku", "ext.sku", "list"},
	}

	for _, c := range cases {
		col, path, elastic, typ := parseFieldMapping(c.k, c.v)
		if col != c.col || path != c.path || elastic != c.elastic || typ != c.typ {
			t.Fatalf("parseFieldMapping(%q, %q) = (%q,%q,%q,%q), want (%q,%q,%q,%q)",
				c.k, c.v, col, path, elastic, typ, c.col, c.path, c.elastic, c.typ)
		}
	}
}

func TestGetByPath(t *testing.T) {
	obj := map[string]interface{}{
		"sku": "A1",
		"info": map[string]interface{}{
			"name": "n1",
		},
	}

	if got := getByPath(obj, "sku"); got != "A1" {
		t.Fatalf("getByPath sku = %v", got)
	}
	if got := getByPath(obj, "info.name"); got != "n1" {
		t.Fatalf("getByPath info.name = %v", got)
	}
	if got := getByPath(obj, "missing"); got != nil {
		t.Fatalf("getByPath missing = %v", got)
	}
}

func TestSetByPath(t *testing.T) {
	data := make(map[string]interface{})
	setByPath(data, "sku", "A1")
	setByPath(data, "data.sku", "B1")
	setByPath(data, "data.info.name", "n1")

	if data["sku"] != "A1" {
		t.Fatalf("sku = %v", data["sku"])
	}

	nested, ok := data["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data not map: %v", data["data"])
	}
	if nested["sku"] != "B1" {
		t.Fatalf("data.sku = %v", nested["sku"])
	}
	info, ok := nested["info"].(map[string]interface{})
	if !ok || info["name"] != "n1" {
		t.Fatalf("data.info.name = %v", nested["info"])
	}
}

func TestApplyFieldTypeList(t *testing.T) {
	got := applyFieldType("a,b,c", fieldTypeList)
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestRuleHasPathMapping(t *testing.T) {
	r1 := &Rule{FieldMapping: map[string]string{"title": "es_title"}}
	if r1.hasPathMapping() {
		t.Fatal("expected false")
	}
	r2 := &Rule{FieldMapping: map[string]string{"ext.sku": "sku"}}
	if !r2.hasPathMapping() {
		t.Fatal("expected true for source path")
	}
	r3 := &Rule{FieldMapping: map[string]string{"title": "data.title"}}
	if !r3.hasPathMapping() {
		t.Fatal("expected true for dest path")
	}
}

func TestPathOnlyColumns(t *testing.T) {
	r := &Rule{FieldMapping: map[string]string{
		"dataJson.sku":     "sku",
		"dataJson.skuName": "skuName",
		"id":               "id",
	}}
	only := r.pathOnlyColumns()
	if !only["dataJson"] {
		t.Fatal("dataJson should be path-only")
	}
	if only["id"] {
		t.Fatal("id is exact mapping, not path-only")
	}

	r2 := &Rule{FieldMapping: map[string]string{
		"dataJson":         "dataJson",
		"dataJson.sku":     "sku",
	}}
	only2 := r2.pathOnlyColumns()
	if only2["dataJson"] {
		t.Fatal("exact mapping present, should keep raw JSON column")
	}
}
