package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var msPythonQuery = []byte(`{
	"flags": 439,
    "filters": [{
        "criteria": [
            {"filterType": 4, "value": "f1f59ae4-9318-4f3c-a9b5-81b2eaa5f8a5"},
            {"filterType": 12, "value": "4096"}
        ],
        "pageNumber": 1,
        "pageSize": 1
    }]
}`)

func queryVersions(t *testing.T, srv *httptest.Server, prefix, duration string) []map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost,
		srv.URL+"/"+prefix+"/"+duration+"/extensionquery",
		bytes.NewReader(msPythonQuery),
	)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json;api-version=3.0-preview.1")
	req.Header.Set("Origin", "vscode-file://vscode-app")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var q map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&q); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	versions := q["results"].([]any)[0].(map[string]any)["extensions"].([]any)[0].(map[string]any)["versions"].([]any)
	out := make([]map[string]any, len(versions))
	for i, v := range versions {
		out[i] = v.(map[string]any)
	}
	return out
}

func TestCooldown(t *testing.T) {
	t.Parallel()

	mktplaces := []string{"vscode", "openvsx"}

	const (
		newAge = "0d"
		oldAge = "180d"
	)

	for _, m := range mktplaces {
		t.Run(m, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(newHandler())
			defer srv.Close()

			vNew := queryVersions(t, srv, m, newAge)
			vOld := queryVersions(t, srv, m, oldAge)

			t.Logf("%s versions (%d): %v", newAge, len(vNew), vNew)
			t.Logf("%s versions (%d): %v", oldAge, len(vOld), vOld)

			if len(vNew) == 0 {
				t.Fatalf("%s returned no versions", newAge)
			}
			if len(vOld) == 0 {
				t.Fatalf("%s returned no versions", oldAge)
			}

			parse := func(v map[string]any) time.Time {
				t, _ := time.Parse(time.RFC3339, v["lastUpdated"].(string))
				return t
			}
			dNew := parse(vNew[0])
			dOld := parse(vOld[0])
			t.Logf("%s latest: %s", newAge, dNew)
			t.Logf("%s latest: %s", oldAge, dOld)
			if !dNew.After(dOld) {
				t.Errorf("%s latest (%s) should be more recent than %s latest (%s)", newAge, dNew, oldAge, dOld)
			}
		})
	}
}
