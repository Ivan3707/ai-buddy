package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type chatRequest struct {
	Message string `json:"message"`
}

type source struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type chatResponse struct {
	Answer  string   `json:"answer"`
	Sources []source `json:"sources"`
	Mode    string   `json:"mode"`
}

func chatHandler(aiURL string) http.Handler {
	client := &http.Client{Timeout: 10 * time.Second}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16384)

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var body chatRequest
		if err := decoder.Decode(&body); err != nil {
			send(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			send(w, http.StatusBadRequest, map[string]string{"error": "expected one JSON object"})
			return
		}
		if strings.TrimSpace(body.Message) == "" ||
			utf8.RuneCountInString(body.Message) > 2000 {
			send(w, http.StatusBadRequest, map[string]string{
				"error": "message must contain 1..2000 characters",
			})
			return
		}

		payload, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(
			r.Context(),
			http.MethodPost,
			aiURL+"/chat",
			bytes.NewReader(payload),
		)
		if err != nil {
			send(w, http.StatusBadGateway, map[string]string{"error": "AI unavailable"})
			return
		}

		req.Header.Set("Content-Type", "application/json")

		res, err := client.Do(req)
		if err != nil {
			send(w, http.StatusBadGateway, map[string]string{"error": "AI unavailable"})
			return
		}
		defer res.Body.Close()

		responseBody, readErr := io.ReadAll(io.LimitReader(res.Body, 65537))
		var reply chatResponse
		decoder = json.NewDecoder(bytes.NewReader(responseBody))
		if res.StatusCode != http.StatusOK ||
			readErr != nil ||
			len(responseBody) > 65536 ||
			decoder.Decode(&reply) != nil ||
			decoder.Decode(&struct{}{}) != io.EOF ||
			reply.Answer == "" ||
			(reply.Mode != "mock" && reply.Mode != "llm") || // AiMode in contracts: mock (offline) or llm (real model)
			reply.Sources == nil {
			send(w, http.StatusBadGateway, map[string]string{"error": "invalid AI response"})
			return
		}

		send(w, http.StatusOK, reply)
	})
}
