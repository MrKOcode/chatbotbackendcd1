package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// Structs to represent request and response payloads
type ChatGPTRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// Message here represents a chat message formatted for openai api
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatGPTResponse struct {
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message Message `json:"message"`
}

var chatGPTAPIURL = "https://api.openai.com/v1/chat/completions"
var chatGPTHTTPClient = &http.Client{}

// GetChatGPTResponse interacts with the OpenAI API and retrieves the response
func GetChatGPTResponse(messages []Message) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY is not set in environment variables")
	}

	// OpenAI API URL
	// Construct the request payload
	requestPayload := ChatGPTRequest{
		Model:    "gpt-4",
		Messages: messages,
	}

	// Serialize the request payload to JSON
	requestBody, err := json.Marshal(requestPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request payload: %w", err)
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", chatGPTAPIURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Make the HTTP request
	resp, err := chatGPTHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Do not include the provider response body here. It may contain request
	// details, user content, or other sensitive information and this error can
	// be returned to callers or written to application logs.
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI API error (status %d)", resp.StatusCode)
	}

	// Parse the response JSON
	var chatResponse ChatGPTResponse
	err = json.Unmarshal(responseBody, &chatResponse)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal OpenAI response: %w", err)
	}

	// Return the content of the first choice
	if len(chatResponse.Choices) > 0 {
		return chatResponse.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("no response received from ChatGPT API")
}
