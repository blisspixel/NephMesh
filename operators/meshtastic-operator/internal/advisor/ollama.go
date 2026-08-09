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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaClient calls a local Ollama server's generate endpoint. It is the edge
// path: the model runs on the sensor host itself, so the advisor works with no
// cloud and no network beyond localhost. It requests JSON-formatted output at a
// low temperature so the reply is a structured recommendation, not prose.
type OllamaClient struct {
	BaseURL string
	Model   string
	HTTP    *http.Client
	// NumGPU, when non-negative, sets how many layers Ollama offloads to the GPU.
	// 0 forces CPU inference, which lets a capable model run in system RAM on a
	// memory-constrained edge device (a Jetson shares RAM between CPU and GPU, so
	// a model that will not fit the GPU alongside an SDR still fits on the CPU).
	// A negative value leaves the decision to Ollama.
	NumGPU int
}

// NewOllama builds a client for the given base URL (for example
// http://localhost:11434) and model (for example llama3.2:3b).
func NewOllama(baseURL, model string) *OllamaClient {
	return &OllamaClient{
		BaseURL: baseURL,
		Model:   model,
		HTTP:    &http.Client{Timeout: 180 * time.Second},
		NumGPU:  -1,
	}
}

type ollamaRequest struct {
	Model   string         `json:"model"`
	System  string         `json:"system"`
	Prompt  string         `json:"prompt"`
	Stream  bool           `json:"stream"`
	Format  string         `json:"format"`
	Options map[string]any `json:"options"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Error    string `json:"error"`
}

// Complete sends the prompts to Ollama and returns the model's reply text.
func (o *OllamaClient) Complete(ctx context.Context, system, user string) (string, error) {
	options := map[string]any{"temperature": 0.2}
	if o.NumGPU >= 0 {
		options["num_gpu"] = o.NumGPU
	}
	body, err := json.Marshal(ollamaRequest{
		Model:   o.Model,
		System:  system,
		Prompt:  user,
		Stream:  false,
		Format:  "json",
		Options: options,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama status %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	var or ollamaResponse
	if err := json.Unmarshal(data, &or); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}
	if or.Error != "" {
		return "", fmt.Errorf("ollama error: %s", or.Error)
	}
	return or.Response, nil
}
