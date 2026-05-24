package repository

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
	return "http://localhost:2003/v1/files/" + str
}
