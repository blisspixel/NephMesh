/*
Copyright 2026 The NephMesh Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package advisor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaCompleteRoundTrip(t *testing.T) {
	var gotReq ollamaRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/generate", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotReq))
		_ = json.NewEncoder(w).Encode(ollamaResponse{Response: `{"action":"hold","rationale":"quiet","confidence":"high"}`})
	}))
	defer srv.Close()

	c := NewOllama(srv.URL, "llama3.2:3b")
	out, err := c.Complete(context.Background(), "sys", "user")
	require.NoError(t, err)
	assert.Contains(t, out, `"action":"hold"`)
	// It requests deterministic JSON output from the named model.
	assert.Equal(t, "llama3.2:3b", gotReq.Model)
	assert.Equal(t, "json", gotReq.Format)
	assert.False(t, gotReq.Stream)
	assert.Equal(t, "sys", gotReq.System)
}

func TestOllamaNumGPUForcesCPUOption(t *testing.T) {
	var opts map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Options map[string]any `json:"options"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		opts = req.Options
		_ = json.NewEncoder(w).Encode(ollamaResponse{Response: `{"action":"hold","rationale":"x","confidence":"low"}`})
	}))
	defer srv.Close()

	c := NewOllama(srv.URL, "m")
	c.NumGPU = 0
	_, err := c.Complete(context.Background(), "s", "u")
	require.NoError(t, err)
	assert.EqualValues(t, 0, opts["num_gpu"], "num_gpu 0 (CPU) is passed through")
}

func TestOllamaNon200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("model not loaded"))
	}))
	defer srv.Close()
	_, err := NewOllama(srv.URL, "m").Complete(context.Background(), "s", "u")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

func TestOllamaErrorFieldIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(ollamaResponse{Error: "model 'nope' not found"})
	}))
	defer srv.Close()
	_, err := NewOllama(srv.URL, "nope").Complete(context.Background(), "s", "u")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestAdviseThroughOllamaEndToEnd wires the Advisor to a fake Ollama server so the
// whole path (prompt build, HTTP, parse, validate) is covered without a model.
func TestAdviseThroughOllamaEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(ollamaResponse{Response: `{"action":"change_preset","targetPreset":"MEDIUM_SLOW","rationale":"lower airtime","confidence":"medium"}`})
	}))
	defer srv.Close()

	rec, _, err := New(NewOllama(srv.URL, "m")).Advise(context.Background(), sampleSituation())
	require.NoError(t, err)
	assert.Equal(t, ActionChangePreset, rec.Action)
	assert.Equal(t, "MEDIUM_SLOW", rec.TargetPreset)
}
