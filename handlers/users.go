package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"
	"masterboxer.com/project-micro-journal/models"
)

func GetUsers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`SELECT id, username, display_name, dob, 
            gender, email, password, created_at FROM users`)
		if err != nil {
			http.Error(w, "Database query failed", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		defer rows.Close()

		var users []models.User
		for rows.Next() {
			var u models.User
			if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.DOB,
				&u.Gender, &u.Email, &u.Password, &u.CreatedAt); err != nil {
				http.Error(w, "Error scanning user data", http.StatusInternalServerError)
				log.Println(err)
				return
			}
			u.Password = ""
			users = append(users, u)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "Error iterating rows", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		json.NewEncoder(w).Encode(users)
	}
}

func GetUserById(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]

		var u models.User
		err := db.QueryRow(`SELECT id, username, display_name, dob, 
            gender, email, password, created_at FROM users WHERE id = $1`, id).
			Scan(&u.ID, &u.Username, &u.DisplayName, &u.DOB, &u.Gender, &u.Email,
				&u.Password, &u.CreatedAt)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "User not found", http.StatusNotFound)
			} else {
				http.Error(w, "Database query failed", http.StatusInternalServerError)
				log.Println(err)
			}
			return
		}

		u.Password = ""
		json.NewEncoder(w).Encode(u)
	}
}

func DeleteUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]

		var u models.User
		err := db.QueryRow("SELECT id, username, email FROM users WHERE id = $1", id).
			Scan(&u.ID, &u.Username, &u.Email)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "User not found", http.StatusNotFound)
			} else {
				http.Error(w, "Database query failed", http.StatusInternalServerError)
				log.Println(err)
			}
			return
		}

		_, err = db.Exec("DELETE FROM users WHERE id = $1", id)
		if err != nil {
			http.Error(w, "Failed to delete user", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"message": "User deleted successfully"})
	}
}

func CreateUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var u models.User
		if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if u.Username == "" || u.DisplayName == "" || u.Email == "" || u.Password == "" {
			http.Error(w, "Username, display_name, email, and password are required", http.StatusBadRequest)
			return
		}

		if time.Time(u.DOB).IsZero() {
			http.Error(w, "Date of birth is required", http.StatusBadRequest)
			return
		}

		if time.Time(u.DOB).After(time.Now()) {
			http.Error(w, "Date of birth cannot be in the future", http.StatusBadRequest)
			return
		}

		if u.Gender == "" {
			http.Error(w, "Gender is required", http.StatusBadRequest)
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Failed to hash password", http.StatusInternalServerError)
			return
		}

		err = db.QueryRow(
			`INSERT INTO users (username, display_name, dob, gender, email, password, created_at) 
            VALUES ($1, $2, $3, $4, $5, $6, NOW()) RETURNING id, created_at`,
			u.Username, u.DisplayName, u.DOB, u.Gender, u.Email, string(hashedPassword),
		).Scan(&u.ID, &u.CreatedAt)

		if err != nil {
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		u.Password = ""
		json.NewEncoder(w).Encode(u)
	}
}

func UpdateUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var u models.User
		if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		vars := mux.Vars(r)
		id := vars["id"]

		setClauses := []string{}
		args := []interface{}{}
		i := 1

		if u.Username != "" {
			setClauses = append(setClauses, "username = $"+strconv.Itoa(i))
			args = append(args, u.Username)
			i++
		}
		if u.DisplayName != "" {
			setClauses = append(setClauses, "display_name = $"+strconv.Itoa(i))
			args = append(args, u.DisplayName)
			i++
		}
		if u.Email != "" {
			setClauses = append(setClauses, "email = $"+strconv.Itoa(i))
			args = append(args, u.Email)
			i++
		}
		if !time.Time(u.DOB).IsZero() {
			if time.Time(u.DOB).After(time.Now()) {
				http.Error(w, "Date of birth cannot be in the future", http.StatusBadRequest)
				return
			}
			setClauses = append(setClauses, "dob = $"+strconv.Itoa(i))
			args = append(args, u.DOB)
			i++
		}
		if u.Gender != "" {
			setClauses = append(setClauses, "gender = $"+strconv.Itoa(i))
			args = append(args, u.Gender)
			i++
		}

		if len(setClauses) == 0 {
			http.Error(w, "No fields provided for update", http.StatusBadRequest)
			return
		}

		sqlStr := "UPDATE users SET " + strings.Join(setClauses, ", ") +
			" WHERE id = $" + strconv.Itoa(i)
		args = append(args, id)

		_, err := db.Exec(sqlStr, args...)
		if err != nil {
			http.Error(w, "Database update failed", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		var updatedUser models.User
		err = db.QueryRow(`SELECT id, username, display_name, dob, 
            gender, email, password, created_at FROM users WHERE id = $1`, id).
			Scan(&updatedUser.ID, &updatedUser.Username, &updatedUser.DisplayName,
				&updatedUser.DOB, &updatedUser.Gender, &updatedUser.Email,
				&updatedUser.Password, &updatedUser.CreatedAt)

		if err != nil {
			http.Error(w, "Failed to fetch updated user", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		updatedUser.Password = ""
		json.NewEncoder(w).Encode(updatedUser)
	}
}

func GetUserBuddies(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])

		rows, err := db.Query(`
            SELECT u.id, u.username, u.display_name 
            FROM buddies b 
            JOIN users u ON b.buddy_id = u.id 
            WHERE b.user_id = $1`, userID)
		if err != nil {
			http.Error(w, "Database query failed", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		defer rows.Close()

		var buddies []models.UserBuddies
		for rows.Next() {
			var b models.UserBuddies
			if err := rows.Scan(&b.ID, &b.Username, &b.DisplayName); err != nil {
				http.Error(w, "Error scanning buddy data", http.StatusInternalServerError)
				log.Println(err)
				return
			}
			buddies = append(buddies, b)
		}

		json.NewEncoder(w).Encode(buddies)
	}
}

func RemoveBuddy(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])
		buddyID, _ := strconv.Atoi(vars["buddy_id"])

		result, err := db.Exec("DELETE FROM buddies WHERE user_id = $1 AND buddy_id = $2", userID, buddyID)
		if err != nil {
			http.Error(w, "Failed to remove buddy", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		if rowsAffected, _ := result.RowsAffected(); rowsAffected == 0 {
			http.Error(w, "Buddy relationship not found", http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"message": "Buddy removed successfully"})
	}
}

func SearchUsers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			http.Error(w, "Search query 'q' parameter is required", http.StatusBadRequest)
			return
		}

		if len(query) > 50 {
			query = query[:50]
		}

		rows, err := db.Query(`
			SELECT id, username, display_name, dob, gender, email, created_at
			FROM users 
			WHERE username ILIKE $1 
			   OR display_name ILIKE $1
			ORDER BY 
				-- Prioritize exact matches first, then partial
				CASE WHEN username ILIKE $2 THEN 0 ELSE 1 END +
				CASE WHEN display_name ILIKE $2 THEN 0 ELSE 1 END,
				-- Then by relevance (shorter distance to search term)
				LENGTH(username) - LENGTH($1),
				LENGTH(display_name) - LENGTH($1)
			LIMIT 20`,
			"%"+query+"%",
			query+"%")
		if err != nil {
			http.Error(w, "Database search failed", http.StatusInternalServerError)
			log.Println("SearchUsers error:", err)
			return
		}
		defer rows.Close()

		var users []models.UserSearchResult
		for rows.Next() {
			var u models.UserSearchResult
			if err := rows.Scan(
				&u.ID,
				&u.Username,
				&u.DisplayName,
				&u.DOB,
				&u.Gender,
				&u.Email,
				&u.CreatedAt); err != nil {
				http.Error(w, "Error scanning search results", http.StatusInternalServerError)
				log.Println(err)
				return
			}
			users = append(users, u)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
	}
}

func RegisterFCMToken(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req TokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Token == "" {
			http.Error(w, "FCM token is required", http.StatusBadRequest)
			return
		}

		if req.UserID == 0 {
			http.Error(w, "User ID is required", http.StatusBadRequest)
			return
		}

		_, err := db.Exec(`
			INSERT INTO fcm_tokens (user_id, token, created_at, updated_at)
			VALUES ($1, $2, NOW(), NOW())
			ON CONFLICT (user_id, token) 
			DO UPDATE SET updated_at = NOW()`,
			req.UserID, req.Token)

		if err != nil {
			http.Error(w, "Failed to register FCM token", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "FCM token registered successfully",
		})
	}
}

type TokenRequest struct {
	Token     string `json:"token"`
	UserID    int    `json:"user_id"`
	Timestamp string `json:"timestamp,omitempty"`
}
