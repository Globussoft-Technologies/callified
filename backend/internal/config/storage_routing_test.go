package config

import "testing"

func TestStorageRoutingFromPublicServerSubdomain(t *testing.T) {
	for _, tc := range []struct {
		url string
		nas bool
	}{
		{"https://testgo2.callified.ai", true},
		{"https://qa-test.callified.ai", true},
		{"https://dev.testing.callified.ai", true},
		{"HTTPS://TESTGO2.callified.ai:443/recordings", true},
		{"testgo2.callified.ai", true},
		{"https://testgo2.callified.ai.", true},
		{"https://app.callified.ai", false},
		{"https://app.callified.ai/test?environment=test", false},
		{"https://test:password@app.callified.ai", false},
		{"https://callified.ai", false},
		{"https://app.testdomain.ai", false},
		{"http://127.0.0.1:8001", false},
		{"", false},
	} {
		t.Run(tc.url, func(t *testing.T) {
			cfg := Config{PublicServerURL: tc.url}
			if got := cfg.IsTestDeployment(); got != tc.nas {
				t.Fatalf("IsTestDeployment()=%v, want %v", got, tc.nas)
			}
		})
	}
}
