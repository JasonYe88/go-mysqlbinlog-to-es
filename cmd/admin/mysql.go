package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

type ColumnInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Null     string `json:"null"`
	Key      string `json:"key"`
	Comment  string `json:"comment"`
	IsJSON   bool   `json:"is_json"`
	JSONKeys []string `json:"json_keys,omitempty"`
}

func openMySQL(c *Config) (*sql.DB, error) {
	db, err := sql.Open("mysql", mysqlDSN(c))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func listSchemas(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`
		SELECT SCHEMA_NAME FROM information_schema.SCHEMATA
		WHERE SCHEMA_NAME NOT IN ('information_schema','mysql','performance_schema','sys')
		ORDER BY SCHEMA_NAME`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func listTables(db *sql.DB, schema string) ([]string, error) {
	q := `
		SELECT TABLE_NAME FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'
		ORDER BY TABLE_NAME`
	rows, err := db.Query(q, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func listColumns(db *sql.DB, schema, table string) ([]ColumnInfo, error) {
	q := `
		SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY, COLUMN_COMMENT, DATA_TYPE
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION`
	rows, err := db.Query(q, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ColumnInfo
	for rows.Next() {
		var c ColumnInfo
		var dataType string
		if err := rows.Scan(&c.Name, &c.Type, &c.Null, &c.Key, &c.Comment, &dataType); err != nil {
			return nil, err
		}
		c.IsJSON = strings.EqualFold(dataType, "json")
		out = append(out, c)
	}
	return out, rows.Err()
}

func sampleJSONKeys(db *sql.DB, schema, table, column string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 20
	}
	q := fmt.Sprintf(
		"SELECT %s FROM %s.%s WHERE %s IS NOT NULL LIMIT ?",
		quoteIdent(column), quoteIdent(schema), quoteIdent(table), quoteIdent(column),
	)
	rows, err := db.Query(q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keySet := map[string]struct{}{}
	for rows.Next() {
		var raw sql.RawBytes
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			continue
		}
		var v interface{}
		if err := json.Unmarshal(raw, &v); err != nil {
			continue
		}
		collectJSONPaths(v, "", keySet)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func collectJSONPaths(v interface{}, prefix string, out map[string]struct{}) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, child := range t {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			out[path] = struct{}{}
			collectJSONPaths(child, path, out)
		}
	case []interface{}:
		// mark array itself; do not explode every index
		if prefix != "" {
			out[prefix] = struct{}{}
		}
	default:
		if prefix != "" {
			out[prefix] = struct{}{}
		}
	}
}
