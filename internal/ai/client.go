package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

type Client struct {
	apiKey string
	url    string
}

func NewClient() *Client {
	apiKey := os.Getenv("GEMINI_API_KEY")
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-2.5-flash" // Sensible default
	}

	// Dynamic endpoint construction
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)

	if apiKey == "" {
		log.Println("WARNING: GEMINI_API_KEY not found. AI features will be disabled/mocked.")
	}

	return &Client{
		apiKey: apiKey,
		url:    url,
	}
}

// AnalyzePortfolio sends the snapshot to Gemini and parses the response.
func (c *Client) AnalyzePortfolio(systemInstruction string, snapshot PortfolioSnapshot) (*AIAnalysis, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("AI client not configured")
	}

	// Prepare Payload
	snapJSON, _ := json.Marshal(snapshot)

	// Construct the prompt payload for Gemini REST API
	// We use a simplified structure for the HTTP request
	payload := map[string]interface{}{
		"system_instruction": map[string]interface{}{
			"parts": map[string]interface{}{
				"text": systemInstruction,
			},
		},
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": fmt.Sprintf("Analyze this portfolio state: %s", string(snapJSON))},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"response_mime_type": "application/json",
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.url+"?key="+c.apiKey, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AI API error %d: %s", resp.StatusCode, string(body))
	}

	// Parse Response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Extract text from Gemini response structure
	// candidates[0].content.parts[0].text
	// This is a bit verbose in Go map[string]interface{}, skipping strict struct for brevity
	candidates, ok := result["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return nil, fmt.Errorf("no candidates in AI response")
	}
	candidate := candidates[0].(map[string]interface{})
	content := candidate["content"].(map[string]interface{})
	parts := content["parts"].([]interface{})
	text := parts[0].(map[string]interface{})["text"].(string)

	// Unmarshal the JSON text inside the response
	var analysis AIAnalysis
	if err := json.Unmarshal([]byte(text), &analysis); err != nil {
		return nil, fmt.Errorf("failed to parse AI JSON output: %v. Raw: %s", err, text)
	}

	return &analysis, nil
}
