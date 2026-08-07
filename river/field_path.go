package river

import (
	"strings"
)

// parseFieldMapping parses a [rule.field] entry.
//
// Source key examples:
//
//	title          -> mysql column "title"
//	ext.sku        -> mysql column "ext", JSON path "sku"
//	ext.info.name  -> mysql column "ext", JSON path "info.name"
//
// Dest value examples:
//
//	sku            -> ES field "sku"
//	data.sku       -> ES nested field data.sku
//	es_tags,list   -> ES field "es_tags" with list modifier
//	,list          -> keep source key as ES field name + list modifier
func parseFieldMapping(k string, v string) (mysqlCol, mysqlPath, elasticPath, fieldType string) {
	composedField := strings.Split(v, ",")

	elasticPath = composedField[0]
	if len(composedField) >= 2 {
		fieldType = composedField[1]
	}

	if i := strings.Index(k, "."); i >= 0 {
		mysqlCol = k[:i]
		mysqlPath = k[i+1:]
	} else {
		mysqlCol = k
	}

	if len(elasticPath) == 0 {
		elasticPath = k
	}

	return mysqlCol, mysqlPath, elasticPath, fieldType
}

func (r *Rule) hasPathMapping() bool {
	for k, v := range r.FieldMapping {
		if strings.Contains(k, ".") {
			return true
		}
		dest := strings.Split(v, ",")[0]
		if strings.Contains(dest, ".") {
			return true
		}
	}
	return false
}

// pathOnlyColumns returns MySQL columns that appear only in JSON path mappings
// (e.g. "dataJson.sku") and have no exact column mapping (e.g. dataJson = "...").
// Those columns are used for extraction but the raw JSON object is not written to ES.
func (r *Rule) pathOnlyColumns() map[string]bool {
	exact := map[string]bool{}
	pathRoot := map[string]bool{}
	for k := range r.FieldMapping {
		mysqlCol, mysqlPath, _, _ := parseFieldMapping(k, r.FieldMapping[k])
		if mysqlPath == "" {
			exact[mysqlCol] = true
		} else {
			pathRoot[mysqlCol] = true
		}
	}
	out := map[string]bool{}
	for col := range pathRoot {
		if !exact[col] {
			out[col] = true
		}
	}
	return out
}

// getByPath walks a JSON object by dotted path.
// value should usually be map[string]interface{} from json.Unmarshal.
func getByPath(value interface{}, path string) interface{} {
	if path == "" {
		return value
	}

	cur := value
	for _, p := range strings.Split(path, ".") {
		if cur == nil {
			return nil
		}
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil
		}
		cur = m[p]
	}
	return cur
}

// setByPath writes value into data by dotted path, creating intermediate maps.
func setByPath(data map[string]interface{}, path string, value interface{}) {
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		data[parts[0]] = value
		return
	}

	cur := data
	for i := 0; i < len(parts)-1; i++ {
		p := parts[i]
		next, ok := cur[p]
		if !ok {
			m := make(map[string]interface{})
			cur[p] = m
			cur = m
			continue
		}
		m, ok := next.(map[string]interface{})
		if !ok {
			m = make(map[string]interface{})
			cur[p] = m
		}
		cur = m
	}
	cur[parts[len(parts)-1]] = value
}

// applyFieldType applies list/date modifiers to an already-extracted value.
func applyFieldType(value interface{}, fieldType string) interface{} {
	switch fieldType {
	case fieldTypeList:
		if str, ok := value.(string); ok {
			return strings.Split(str, ",")
		}
	}
	return value
}
