package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
)

func RegisterTokenHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req TokenRequest

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("JSON decode error: %v", err)
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Token == "" {
			http.Error(w, "Token is required", http.StatusBadRequest)
			return
		}

		if req.UserID == 0 {
			http.Error(w, "Valid user_id is required", http.StatusBadRequest)
			return
		}

		query := `
			INSERT INTO fcm_tokens (user_id, token, created_at, updated_at)
			VALUES ($1, $2, NOW(), NOW())
			ON CONFLICT (user_id, token) 
			DO UPDATE SET updated_at = NOW()`

		_, err := db.Exec(query, req.UserID, req.Token)

		if err != nil {
			log.Printf("Database error saving FCM token: %v", err)
			http.Error(w, "Failed to register token", http.StatusInternalServerError)
			return
		}

		log.Printf("✅ FCM token registered for user %d", req.UserID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "FCM token registered successfully",
		})
	}
}
