package client

func extractMuteUntil(parsed map[string]any) float64 {
	data, _ := parsed["data"].(map[string]any)
	if data == nil {
		return 0
	}
	bizData, _ := data["biz_data"].(map[string]any)
	if bizData == nil {
		return 0
	}
	switch v := bizData["mute_until"].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return 0
}

func isMuteResponse(bizCode int) bool {
	return bizCode == 14
}
