package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"masterboxer.com/project-micro-journal/services"
)

func SendVerificationEmailHandler(db *sql.DB, mailSvc *services.MailService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID int    `json:"user_id"`
			Email  string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if req.UserID == 0 || req.Email == "" {
			http.Error(w, "user_id and email are required", http.StatusBadRequest)
			return
		}

		var alreadyVerified bool
		err := db.QueryRow(`
			SELECT COALESCE(email_verified, false) FROM users WHERE id = $1
		`, req.UserID).Scan(&alreadyVerified)
		if err == sql.ErrNoRows {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if alreadyVerified {
			http.Error(w, "Email is already verified", http.StatusConflict)
			return
		}

		if err := sendVerificationEmail(db, mailSvc, req.UserID, req.Email); err != nil {
			fmt.Printf("[SendVerification] Failed for %s: %v\n", req.Email, err)
			http.Error(w, "Failed to send verification email", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Verification email sent"))
	}
}

func VerifyEmailHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "Missing token", http.StatusBadRequest)
			return
		}

		var userID int
		var expiresAt time.Time
		var used bool
		err := db.QueryRow(`
			SELECT user_id, expires_at, used
			FROM email_verifications
			WHERE token = $1
		`, token).Scan(&userID, &expiresAt, &used)

		if err == sql.ErrNoRows {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if used {
			http.Error(w, "Token has already been used", http.StatusUnauthorized)
			return
		}
		if time.Now().After(expiresAt) {
			http.Error(w, "Token has expired", http.StatusUnauthorized)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		_, err = tx.Exec(`
			UPDATE users SET email_verified = true, email_verified_at = NOW()
			WHERE id = $1
		`, userID)
		if err != nil {
			http.Error(w, "Failed to verify email", http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec(`
			UPDATE email_verifications SET used = true WHERE token = $1
		`, token)
		if err != nil {
			http.Error(w, "Failed to invalidate token", http.StatusInternalServerError)
			return
		}

		if err = tx.Commit(); err != nil {
			http.Error(w, "Failed to commit changes", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Email verified successfully"))
	}
}

func ResendVerificationEmailHandler(db *sql.DB, mailSvc *services.MailService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
			http.Error(w, "Valid email is required", http.StatusBadRequest)
			return
		}

		var userID int
		var alreadyVerified bool
		err := db.QueryRow(`
			SELECT id, COALESCE(email_verified, false)
			FROM users WHERE email = $1
		`, req.Email).Scan(&userID, &alreadyVerified)

		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("If the email exists and is unverified, a new link has been sent"))
			return
		}
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if alreadyVerified {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("If the email exists and is unverified, a new link has been sent"))
			return
		}

		if err := sendVerificationEmail(db, mailSvc, userID, req.Email); err != nil {
			fmt.Printf("[ResendVerification] Failed for %s: %v\n", req.Email, err)
			http.Error(w, "Failed to send verification email", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("If the email exists and is unverified, a new link has been sent"))
	}
}

func sendVerificationEmail(db *sql.DB, mailSvc *services.MailService, userID int, email string) error {
	_, err := db.Exec(`DELETE FROM email_verifications WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("failed to clear old tokens: %w", err)
	}
	token := generateSecureToken()
	expiresAt := time.Now().Add(24 * time.Hour)
	_, err = db.Exec(`
		INSERT INTO email_verifications (user_id, token, expires_at)
		VALUES ($1, $2, $3)
	`, userID, token, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to store token: %w", err)
	}
	return mailSvc.SendVerificationEmail(email, token)
}
