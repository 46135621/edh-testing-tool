package service

import (
	"encoding/json"
	"testing"
)

func TestAnalysisCanMarshalUnavailableProvider(t *testing.T) {
	value := Analysis{Status: "partial", Results: map[string]ProviderResult{"commandersalt": {Status: "unavailable", Error: &ProviderError{Code: "URL_REQUIRED", Message: "URL required"}}}}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected JSON")
	}
}
