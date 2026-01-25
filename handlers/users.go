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
            gender, email, password, is_private, created_at FROM users`)
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
				&u.Gender, &u.Email, &u.Password, &u.IsPrivate, &u.CreatedAt); err != nil {
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

		requestingUserIDStr := r.URL.Query().Get("requesting_user_id")
		var requestingUserID int
		if requestingUserIDStr != "" {
			requestingUserID, _ = strconv.Atoi(requestingUserIDStr)
		}

		var u models.User
		err := db.QueryRow(`SELECT id, username, display_name, dob, 
            gender, email, password, is_private, created_at FROM users WHERE id = $1`, id).
			Scan(&u.ID, &u.Username, &u.DisplayName, &u.DOB, &u.Gender, &u.Email,
				&u.Password, &u.IsPrivate, &u.CreatedAt)
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

		type UserWithStats struct {
			models.User
			FollowersCount       int    `json:"followers_count"`
			FollowingCount       int    `json:"following_count"`
			PendingRequestsCount int    `json:"pending_requests_count,omitempty"`
			IsFollowing          *bool  `json:"is_following,omitempty"`
			IsFollower           *bool  `json:"is_follower,omitempty"`
			FollowRequestSent    *bool  `json:"follow_request_sent,omitempty"`
			FollowRequestFrom    *bool  `json:"follow_request_from,omitempty"`
			FollowStatus         string `json:"follow_status,omitempty"`
		}

		userWithStats := UserWithStats{User: u}

		err = db.QueryRow(`
			SELECT 
				(SELECT COUNT(*) FROM followers WHERE following_id = $1 AND status = 'accepted') as followers,
				(SELECT COUNT(*) FROM followers WHERE follower_id = $1 AND status = 'accepted') as following`,
			id).Scan(&userWithStats.FollowersCount, &userWithStats.FollowingCount)

		if err != nil {
			log.Println("Error fetching follow stats:", err)
		}

		if requestingUserID > 0 && requestingUserID == u.ID {
			var pendingCount int
			err = db.QueryRow(`
				SELECT COUNT(*) FROM followers 
				WHERE following_id = $1 AND status = 'pending'`,
				id).Scan(&pendingCount)
			if err == nil {
				userWithStats.PendingRequestsCount = pendingCount
			}
		}

		if requestingUserID > 0 && requestingUserID != u.ID {
			var currentStatus sql.NullString
			err = db.QueryRow(`
				SELECT status FROM followers 
				WHERE follower_id = $1 AND following_id = $2`,
				requestingUserID, id).Scan(&currentStatus)

			if err == nil && currentStatus.Valid {
				userWithStats.FollowStatus = currentStatus.String
				isFollowing := (currentStatus.String == "accepted")
				requestSent := (currentStatus.String == "pending")
				userWithStats.IsFollowing = &isFollowing
				userWithStats.FollowRequestSent = &requestSent
			} else {
				userWithStats.FollowStatus = "none"
			}

			var reverseStatus sql.NullString
			err = db.QueryRow(`
				SELECT status FROM followers 
				WHERE follower_id = $1 AND following_id = $2`,
				id, requestingUserID).Scan(&reverseStatus)

			if err == nil && reverseStatus.Valid {
				isFollower := (reverseStatus.String == "accepted")
				requestFrom := (reverseStatus.String == "pending")
				userWithStats.IsFollower = &isFollower
				userWithStats.FollowRequestFrom = &requestFrom
			}
		}

		json.NewEncoder(w).Encode(userWithStats)
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
			`INSERT INTO users (username, display_name, dob, gender, email, password, is_private, created_at) 
            VALUES ($1, $2, $3, $4, $5, $6, $7, NOW()) RETURNING id, created_at`,
			u.Username, u.DisplayName, u.DOB, u.Gender, u.Email, string(hashedPassword), u.IsPrivate,
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

		var reqBody map[string]interface{}
		r.Body.Close()
		if _, hasPrivacy := reqBody["is_private"]; hasPrivacy {
			setClauses = append(setClauses, "is_private = $"+strconv.Itoa(i))
			args = append(args, u.IsPrivate)
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
            gender, email, password, is_private, created_at FROM users WHERE id = $1`, id).
			Scan(&updatedUser.ID, &updatedUser.Username, &updatedUser.DisplayName,
				&updatedUser.DOB, &updatedUser.Gender, &updatedUser.Email,
				&updatedUser.Password, &updatedUser.IsPrivate, &updatedUser.CreatedAt)

		if err != nil {
			http.Error(w, "Failed to fetch updated user", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		updatedUser.Password = ""
		json.NewEncoder(w).Encode(updatedUser)
	}
}

func UpdateUserPrivacy(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]

		var req struct {
			IsPrivate bool `json:"is_private"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		_, err := db.Exec("UPDATE users SET is_private = $1 WHERE id = $2", req.IsPrivate, id)
		if err != nil {
			http.Error(w, "Failed to update privacy setting", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":    "Privacy setting updated",
			"is_private": req.IsPrivate,
		})
	}
}

func SearchUsers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			http.Error(w, "Search query 'q' parameter is required", http.StatusBadRequest)
			return
		}

		requestingUserIDStr := r.URL.Query().Get("requesting_user_id")
		var requestingUserID int
		if requestingUserIDStr != "" {
			requestingUserID, _ = strconv.Atoi(requestingUserIDStr)
		}

		if len(query) > 50 {
			query = query[:50]
		}

		var rows *sql.Rows
		var err error

		if requestingUserID > 0 {
			rows, err = db.Query(`
				SELECT 
					u.id, u.username, u.display_name, u.dob, u.gender, u.email, u.is_private, u.created_at,
					COALESCE((SELECT status FROM followers WHERE follower_id = $3 AND following_id = u.id), 'none') as follow_status,
					EXISTS(SELECT 1 FROM followers WHERE follower_id = u.id AND following_id = $3 AND status = 'accepted') as is_follower
				FROM users u
				WHERE (u.username ILIKE $1 OR u.display_name ILIKE $1)
				  AND u.id != $3
				ORDER BY 
					CASE WHEN u.username ILIKE $2 THEN 0 ELSE 1 END +
					CASE WHEN u.display_name ILIKE $2 THEN 0 ELSE 1 END,
					LENGTH(u.username) - LENGTH($1),
					LENGTH(u.display_name) - LENGTH($1)
				LIMIT 20`,
				"%"+query+"%", query+"%", requestingUserID)
		} else {
			rows, err = db.Query(`
				SELECT id, username, display_name, dob, gender, email, is_private, created_at
				FROM users 
				WHERE username ILIKE $1 OR display_name ILIKE $1
				ORDER BY 
					CASE WHEN username ILIKE $2 THEN 0 ELSE 1 END +
					CASE WHEN display_name ILIKE $2 THEN 0 ELSE 1 END,
					LENGTH(username) - LENGTH($1),
					LENGTH(display_name) - LENGTH($1)
				LIMIT 20`,
				"%"+query+"%", query+"%")
		}

		if err != nil {
			http.Error(w, "Database search failed", http.StatusInternalServerError)
			log.Println("SearchUsers error:", err)
			return
		}
		defer rows.Close()

		type UserSearchResultWithFollow struct {
			models.UserSearchResult
			IsPrivate         bool   `json:"is_private"`
			IsFollowing       *bool  `json:"is_following,omitempty"`
			IsFollower        *bool  `json:"is_follower,omitempty"`
			FollowRequestSent *bool  `json:"follow_request_sent,omitempty"`
			FollowStatus      string `json:"follow_status,omitempty"`
		}

		var users []UserSearchResultWithFollow
		for rows.Next() {
			var u UserSearchResultWithFollow

			if requestingUserID > 0 {
				var followStatus string
				var isFollower bool
				if err := rows.Scan(
					&u.ID,
					&u.Username,
					&u.DisplayName,
					&u.DOB,
					&u.Gender,
					&u.Email,
					&u.IsPrivate,
					&u.CreatedAt,
					&followStatus,
					&isFollower); err != nil {
					http.Error(w, "Error scanning search results", http.StatusInternalServerError)
					log.Println(err)
					return
				}
				u.FollowStatus = followStatus
				isFollowing := (followStatus == "accepted")
				requestSent := (followStatus == "pending")
				u.IsFollowing = &isFollowing
				u.FollowRequestSent = &requestSent
				u.IsFollower = &isFollower
			} else {
				if err := rows.Scan(
					&u.ID,
					&u.Username,
					&u.DisplayName,
					&u.DOB,
					&u.Gender,
					&u.Email,
					&u.IsPrivate,
					&u.CreatedAt); err != nil {
					http.Error(w, "Error scanning search results", http.StatusInternalServerError)
					log.Println(err)
					return
				}
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
