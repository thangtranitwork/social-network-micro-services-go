package service

import (
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
)

func getStringVal(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case dbtype.Date:
		return v.Time().Format("2006-01-02")
	case dbtype.LocalDateTime:
		return v.Time().Format(time.RFC3339)
	case time.Time:
		return v.Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func getIntVal(val interface{}) int {
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func getStringSliceVal(val interface{}) []string {
	result := make([]string, 0)
	if val == nil {
		return result
	}
	switch v := val.(type) {
	case []interface{}:
		for _, item := range v {
			if str, ok := item.(string); ok && str != "" {
				result = append(result, str)
			}
		}
		return result
	case []string:
		for _, str := range v {
			if str != "" {
				result = append(result, str)
			}
		}
		return result
	default:
		return result
	}
}
