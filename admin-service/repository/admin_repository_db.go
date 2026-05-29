package repository

import (
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
)

func getStringVal(val interface{}) string {
	if val == nil {
		return ""
	}
	if v, ok := val.(string); ok {
		return v
	}
	return ""
}

func getIntVal(val interface{}) int {
	if val == nil {
		return 0
	}
	if v, ok := val.(int64); ok {
		return int(v)
	}
	return 0
}

func parseTime(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, s)
	}
	return t
}

func getTimeVal(v interface{}) time.Time {
	if v == nil {
		return time.Time{}
	}
	switch val := v.(type) {
	case dbtype.Date:
		return val.Time()
	case dbtype.LocalDateTime:
		return val.Time()
	case time.Time:
		return val
	case string:
		return parseTime(val)
	default:
		return time.Time{}
	}
}

func formatFileURL(id interface{}) string {
	if id == nil {
		return ""
	}
	str, ok := id.(string)
	if !ok || str == "" {
		return ""
	}
	if len(str) > 4 && str[:4] == "http" {
		return str
	}
	return "http://localhost:11111/v1/files/" + str
}
