package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/idtoken"
	"masterboxer.com/project-micro-journal/models"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

var accessSecretKey = []byte("access-secret-key")
var refreshSecretKey = []byte("refresh-secret-key")

func createAccessToken(email string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"email": email,
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
	})
	return token.SignedString(accessSecretKey)
}

func createRefreshToken(email string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"email": email,
		"exp":   time.Now().Add(7 * 24 * time.Hour).Unix(),
		"jti":   fmt.Sprintf("%d", time.Now().UnixNano()),
	})
	return token.SignedString(refreshSecretKey)
}

func GoogleSignInHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IDToken string `json:"id_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		payload, err := idtoken.Validate(r.Context(), req.IDToken, "1056025366422-ek3d7gljf740ej7lbm3f9bu2ikdpl9at.apps.googleusercontent.com")
		if err != nil {
			http.Error(w, "Invalid Google token: "+err.Error(), http.StatusUnauthorized)
			return
		}

		googleID := payload.Subject
		email, _ := payload.Claims["email"].(string)
		name, _ := payload.Claims["name"].(string)
		picture, _ := payload.Claims["picture"].(string)

		var user models.User
		err = db.QueryRow(
			`SELECT id, username, display_name, email FROM users 
             WHERE google_id = $1 OR email = $2`, googleID, email,
		).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Email)

		// Existing user
		if err == nil {
			_, _ = db.Exec(
				`UPDATE users SET google_id = $1 WHERE id = $2 AND google_id IS NULL`,
				googleID, user.ID,
			)
			accessToken, _ := createAccessToken(user.Email)
			refreshToken, _ := createRefreshToken(user.Email)
			expiresAt := time.Now().Add(7 * 24 * time.Hour)
			_, err = db.Exec(`
                INSERT INTO refresh_tokens (user_id, token, expires_at)
                VALUES ($1, $2, $3)
                ON CONFLICT (token) DO NOTHING`, user.ID, refreshToken, expiresAt)
			if err != nil {
				http.Error(w, "Could not save refresh token", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"needs_onboarding": false,
				"access_token":     accessToken,
				"refresh_token":    refreshToken,
				"user_id":          strconv.Itoa(user.ID),
				"username":         user.Username,
				"display_name":     user.DisplayName,
			})
			return
		}

		// New user
		if err == sql.ErrNoRows {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"needs_onboarding": true,
				"google_id":        googleID,
				"email":            email,
				"display_name":     name,
				"picture":          picture,
			})
			return
		}

		http.Error(w, "Database error", http.StatusInternalServerError)
	}
}

func CompleteGoogleSignUp(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			GoogleID    string `json:"google_id"`
			Email       string `json:"email"`
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			DOB         string `json:"dob"`
			Gender      string `json:"gender"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Validate required fields
		if req.Username == "" || req.DOB == "" || req.Gender == "" {
			http.Error(w, "Username, date of birth, and gender are required", http.StatusBadRequest)
			return
		}

		// Check username isn't taken
		var exists int
		db.QueryRow("SELECT COUNT(*) FROM users WHERE username = $1", req.Username).Scan(&exists)
		if exists > 0 {
			http.Error(w, "Username already taken", http.StatusConflict)
			return
		}

		dob, err := time.Parse("2006-01-02", req.DOB)
		if err != nil {
			http.Error(w, "Invalid date format, use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		if dob.After(time.Now()) {
			http.Error(w, "Date of birth cannot be in the future", http.StatusBadRequest)
			return
		}

		isPrivate := true
		var userID int
		err = db.QueryRow(
			`INSERT INTO users (username, display_name, email, google_id, auth_provider, dob, gender, is_private, created_at)
             VALUES ($1, $2, $3, $4, 'google', $5, $6, $7, NOW()) RETURNING id`,
			req.Username, req.DisplayName, req.Email, req.GoogleID, dob, req.Gender, isPrivate,
		).Scan(&userID)
		if err != nil {
			http.Error(w, "Failed to create user: "+err.Error(), http.StatusInternalServerError)
			return
		}

		accessToken, _ := createAccessToken(req.Email)
		refreshToken, _ := createRefreshToken(req.Email)
		expiresAt := time.Now().Add(7 * 24 * time.Hour)
		db.Exec(`
            INSERT INTO refresh_tokens (user_id, token, expires_at)
            VALUES ($1, $2, $3)
            ON CONFLICT (token) DO NOTHING`, userID, refreshToken, expiresAt)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"needs_onboarding": false,
			"access_token":     accessToken,
			"refresh_token":    refreshToken,
			"user_id":          strconv.Itoa(userID),
			"username":         req.Username,
			"display_name":     req.DisplayName,
		})
	}
}

func VerifyTokenHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
			return
		}

		var tokenString string
		fmt.Sscanf(authHeader, "Bearer %s", &tokenString)

		if err := verifyAccessToken(tokenString); err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Token is valid"))
	}
}

func verifyAccessToken(tokenString string) error {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return accessSecretKey, nil
	})

	if err != nil || !token.Valid {
		return fmt.Errorf("invalid token")
	}
	return nil
}

func LoginHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var loginReq LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		var user models.User
		err := db.QueryRow(`SELECT id, username, display_name, email, password 
			FROM users WHERE email = $1`, loginReq.Email).
			Scan(&user.ID, &user.Username, &user.DisplayName, &user.Email, &user.Password)

		if user.Password == "" {
			http.Error(w, "This account uses Google Sign-In", http.StatusUnauthorized)
			return
		}

		if err != nil {
			http.Error(w, "Invalid email or password", http.StatusUnauthorized)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(loginReq.Password)); err != nil {
			http.Error(w, "Invalid email or password", http.StatusUnauthorized)
			return
		}

		accessToken, err := createAccessToken(user.Email)
		if err != nil {
			http.Error(w, "Could not create access token", http.StatusInternalServerError)
			return
		}

		refreshToken, err := createRefreshToken(user.Email)
		if err != nil {
			http.Error(w, "Could not create refresh token", http.StatusInternalServerError)
			return
		}

		expiresAt := time.Now().Add(7 * 24 * time.Hour)

		_, err = db.Exec(`
			INSERT INTO refresh_tokens (user_id, token, expires_at)
			VALUES ($1, $2, $3)
		`, user.ID, refreshToken, expiresAt)
		if err != nil {
			http.Error(w, "Could not save refresh token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"user_id":       strconv.Itoa(user.ID),
			"username":      user.Username,
			"display_name":  user.DisplayName,
		})
	}
}

func RefreshTokenHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		token, err := jwt.Parse(req.RefreshToken, func(token *jwt.Token) (interface{}, error) {
			return refreshSecretKey, nil
		})
		if err != nil || !token.Valid {
			http.Error(w, "Invalid or expired refresh token", http.StatusUnauthorized)
			return
		}

		claims, _ := token.Claims.(jwt.MapClaims)
		email, ok := claims["email"].(string)
		if !ok {
			http.Error(w, "Invalid token claims", http.StatusUnauthorized)
			return
		}

		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM refresh_tokens WHERE token = $1", req.RefreshToken).Scan(&count)
		if err != nil || count == 0 {
			http.Error(w, "Refresh token not recognized", http.StatusUnauthorized)
			return
		}

		accessToken, err := createAccessToken(email)
		if err != nil {
			http.Error(w, "Failed to create access token", http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"access_token": accessToken,
		})
	}
}

func LogoutHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		if req.RefreshToken == "" {
			http.Error(w, "Missing refresh token", http.StatusBadRequest)
			return
		}

		token, err := jwt.Parse(req.RefreshToken, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return refreshSecretKey, nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "Invalid or expired refresh token", http.StatusUnauthorized)
			return
		}

		result, err := db.Exec("DELETE FROM refresh_tokens WHERE token = $1", req.RefreshToken)
		if err != nil {
			http.Error(w, "Failed to log out", http.StatusInternalServerError)
			return
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			http.Error(w, "Failed to check logout status", http.StatusInternalServerError)
			return
		}

		if rowsAffected == 0 {
			http.Error(w, "Refresh token not found", http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Logged out successfully"))
	}
}
