package hope

import "encoding/json"

func encodeStrings(values []string) (string, error) {
	raw, err := json.Marshal(values)
	return string(raw), err
}

func decodeStrings(raw string) []string {
	var values []string
	_ = json.Unmarshal([]byte(raw), &values)
	return values
}
