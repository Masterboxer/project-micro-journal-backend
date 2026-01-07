package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"masterboxer.com/project-micro-journal/models"
	"masterboxer.com/project-micro-journal/services"
)

// FollowUser - Send follow request (pending if private, accepted if public)
func FollowUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		followerID, _ := strconv.Atoi(vars["user_id"])

		var req struct {
			FollowingID int `json:"following_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.FollowingID == followerID {
			http.Error(w, "Cannot follow yourself", http.StatusBadRequest)
			return
		}

		// Check if target user exists and get privacy status
		var targetExists bool
		var isPrivate bool
		err := db.QueryRow(`
			SELECT EXISTS(SELECT 1 FROM users WHERE id = $1),
			       COALESCE((SELECT is_private FROM users WHERE id = $1), false)`,
			req.FollowingID).Scan(&targetExists, &isPrivate)
		if err != nil || !targetExists {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		// Check if already has a follow relationship (any status)
		var existingStatus string
		err = db.QueryRow(`
			SELECT status FROM followers 
			WHERE follower_id = $1 AND following_id = $2`,
			followerID, req.FollowingID).Scan(&existingStatus)

		if err == nil {
			// Relationship exists
			if existingStatus == "accepted" {
				http.Error(w, "Already following this user", http.StatusConflict)
				return
			} else if existingStatus == "pending" {
				http.Error(w, "Follow request already sent", http.StatusConflict)
				return
			} else if existingStatus == "rejected" {
				// Update rejected to pending
				_, err = db.Exec(`
					UPDATE followers 
					SET status = $1, updated_at = NOW()
					WHERE follower_id = $2 AND following_id = $3`,
					"pending", followerID, req.FollowingID)
				if err != nil {
					http.Error(w, "Failed to resend follow request", http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]string{
					"message": "Follow request sent",
					"status":  "pending",
				})
				return
			}
		} else if err != sql.ErrNoRows {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		// Determine initial status based on privacy
		status := "accepted"
		if isPrivate {
			status = "pending"
		}

		// Create follow relationship
		_, err = db.Exec(`
			INSERT INTO followers (follower_id, following_id, status, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())`,
			followerID, req.FollowingID, status)

		if err != nil {
			http.Error(w, "Failed to follow user", http.StatusInternalServerError)
			log.Println("FollowUser error:", err)
			return
		}

		// Send notification
		if status == "accepted" {
			go notifyNewFollower(db, followerID, req.FollowingID)
		} else {
			go notifyFollowRequest(db, followerID, req.FollowingID)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"message": map[bool]string{true: "Follow request sent", false: "Successfully followed user"}[isPrivate],
			"status":  status,
		})
	}
}

// AcceptFollowRequest - Accept a pending follow request
func AcceptFollowRequest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])         // Person being followed
		followerID, _ := strconv.Atoi(vars["follower_id"]) // Person who sent request

		result, err := db.Exec(`
			UPDATE followers 
			SET status = 'accepted', updated_at = NOW()
			WHERE follower_id = $1 AND following_id = $2 AND status = 'pending'`,
			followerID, userID)

		if err != nil {
			http.Error(w, "Failed to accept follow request", http.StatusInternalServerError)
			log.Println("AcceptFollowRequest error:", err)
			return
		}

		if rowsAffected, _ := result.RowsAffected(); rowsAffected == 0 {
			http.Error(w, "Follow request not found or already processed", http.StatusNotFound)
			return
		}

		go notifyFollowAccepted(db, userID, followerID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Follow request accepted",
		})
	}
}

// RejectFollowRequest - Reject a pending follow request
func RejectFollowRequest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])
		followerID, _ := strconv.Atoi(vars["follower_id"])

		result, err := db.Exec(`
			UPDATE followers 
			SET status = 'rejected', updated_at = NOW()
			WHERE follower_id = $1 AND following_id = $2 AND status = 'pending'`,
			followerID, userID)

		if err != nil {
			http.Error(w, "Failed to reject follow request", http.StatusInternalServerError)
			log.Println("RejectFollowRequest error:", err)
			return
		}

		if rowsAffected, _ := result.RowsAffected(); rowsAffected == 0 {
			http.Error(w, "Follow request not found or already processed", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Follow request rejected",
		})
	}
}

// CancelFollowRequest - Cancel a sent follow request (pending status only)
func CancelFollowRequest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		followerID, _ := strconv.Atoi(vars["user_id"])
		followingID, _ := strconv.Atoi(vars["following_id"])

		result, err := db.Exec(`
			DELETE FROM followers 
			WHERE follower_id = $1 AND following_id = $2 AND status = 'pending'`,
			followerID, followingID)

		if err != nil {
			http.Error(w, "Failed to cancel follow request", http.StatusInternalServerError)
			log.Println("CancelFollowRequest error:", err)
			return
		}

		if rowsAffected, _ := result.RowsAffected(); rowsAffected == 0 {
			http.Error(w, "Follow request not found or already accepted", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Follow request cancelled",
		})
	}
}

// GetPendingFollowRequests - Get list of pending follow requests received
func GetPendingFollowRequests(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])

		rows, err := db.Query(`
			SELECT u.id, u.username, u.display_name, f.created_at
			FROM followers f
			JOIN users u ON f.follower_id = u.id
			WHERE f.following_id = $1 AND f.status = 'pending'
			ORDER BY f.created_at DESC`,
			userID)

		if err != nil {
			http.Error(w, "Failed to fetch pending requests", http.StatusInternalServerError)
			log.Println("GetPendingFollowRequests error:", err)
			return
		}
		defer rows.Close()

		var requests []models.FollowerInfo
		for rows.Next() {
			var req models.FollowerInfo
			if err := rows.Scan(&req.ID, &req.Username,
				&req.DisplayName, &req.FollowedAt); err != nil {
				http.Error(w, "Error scanning requests", http.StatusInternalServerError)
				return
			}
			requests = append(requests, req)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(requests)
	}
}

// GetSentFollowRequests - Get list of pending follow requests sent
func GetSentFollowRequests(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])

		rows, err := db.Query(`
			SELECT u.id, u.username, u.display_name, f.created_at
			FROM followers f
			JOIN users u ON f.following_id = u.id
			WHERE f.follower_id = $1 AND f.status = 'pending'
			ORDER BY f.created_at DESC`,
			userID)

		if err != nil {
			http.Error(w, "Failed to fetch sent requests", http.StatusInternalServerError)
			log.Println("GetSentFollowRequests error:", err)
			return
		}
		defer rows.Close()

		var requests []models.FollowerInfo
		for rows.Next() {
			var req models.FollowerInfo
			if err := rows.Scan(&req.ID, &req.Username,
				&req.DisplayName, &req.FollowedAt); err != nil {
				http.Error(w, "Error scanning requests", http.StatusInternalServerError)
				return
			}
			requests = append(requests, req)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(requests)
	}
}

// UnfollowUser - Unfollow a user (only works for accepted follows)
func UnfollowUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		followerID, _ := strconv.Atoi(vars["user_id"])
		followingID, _ := strconv.Atoi(vars["following_id"])

		result, err := db.Exec(`
			DELETE FROM followers 
			WHERE follower_id = $1 AND following_id = $2 AND status = 'accepted'`,
			followerID, followingID)

		if err != nil {
			http.Error(w, "Failed to unfollow user", http.StatusInternalServerError)
			log.Println("UnfollowUser error:", err)
			return
		}

		if rowsAffected, _ := result.RowsAffected(); rowsAffected == 0 {
			http.Error(w, "Follow relationship not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Successfully unfollowed user",
		})
	}
}

// RemoveFollower - Remove a follower (accepted only)
func RemoveFollower(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])
		followerID, _ := strconv.Atoi(vars["follower_id"])

		result, err := db.Exec(`
			DELETE FROM followers 
			WHERE follower_id = $1 AND following_id = $2 AND status = 'accepted'`,
			followerID, userID)

		if err != nil {
			http.Error(w, "Failed to remove follower", http.StatusInternalServerError)
			log.Println("RemoveFollower error:", err)
			return
		}

		if rowsAffected, _ := result.RowsAffected(); rowsAffected == 0 {
			http.Error(w, "Follower relationship not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Successfully removed follower",
		})
	}
}

// UnfollowAndRemove - Disconnect both ways (accepted only)
func UnfollowAndRemove(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])
		targetUserID, _ := strconv.Atoi(vars["target_user_id"])

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Transaction error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		_, err = tx.Exec(`
			DELETE FROM followers 
			WHERE follower_id = $1 AND following_id = $2 AND status = 'accepted'`,
			userID, targetUserID)

		if err != nil {
			http.Error(w, "Failed to unfollow", http.StatusInternalServerError)
			log.Println("UnfollowAndRemove unfollow error:", err)
			return
		}

		_, err = tx.Exec(`
			DELETE FROM followers 
			WHERE follower_id = $1 AND following_id = $2 AND status = 'accepted'`,
			targetUserID, userID)

		if err != nil {
			http.Error(w, "Failed to remove follower", http.StatusInternalServerError)
			log.Println("UnfollowAndRemove remove error:", err)
			return
		}

		if err = tx.Commit(); err != nil {
			http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Successfully disconnected from user (both directions)",
		})
	}
}

// GetUserFollowers - Get list of accepted followers only
func GetUserFollowers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])

		rows, err := db.Query(`
			SELECT u.id, u.username, u.display_name, f.created_at
			FROM followers f
			JOIN users u ON f.follower_id = u.id
			WHERE f.following_id = $1 AND f.status = 'accepted'
			ORDER BY f.created_at DESC`,
			userID)

		if err != nil {
			http.Error(w, "Failed to fetch followers", http.StatusInternalServerError)
			log.Println("GetUserFollowers error:", err)
			return
		}
		defer rows.Close()

		var followers []models.FollowerInfo
		for rows.Next() {
			var follower models.FollowerInfo
			if err := rows.Scan(&follower.ID, &follower.Username,
				&follower.DisplayName, &follower.FollowedAt); err != nil {
				http.Error(w, "Error scanning followers", http.StatusInternalServerError)
				return
			}
			followers = append(followers, follower)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(followers)
	}
}

// GetUserFollowing - Get list of accepted following only
func GetUserFollowing(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])

		rows, err := db.Query(`
			SELECT u.id, u.username, u.display_name, f.created_at
			FROM followers f
			JOIN users u ON f.following_id = u.id
			WHERE f.follower_id = $1 AND f.status = 'accepted'
			ORDER BY f.created_at DESC`,
			userID)

		if err != nil {
			http.Error(w, "Failed to fetch following", http.StatusInternalServerError)
			log.Println("GetUserFollowing error:", err)
			return
		}
		defer rows.Close()

		var following []models.FollowerInfo
		for rows.Next() {
			var user models.FollowerInfo
			if err := rows.Scan(&user.ID, &user.Username,
				&user.DisplayName, &user.FollowedAt); err != nil {
				http.Error(w, "Error scanning following", http.StatusInternalServerError)
				return
			}
			following = append(following, user)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(following)
	}
}

// GetFollowStats - Get follower and following counts (accepted only)
func GetFollowStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])

		var stats struct {
			FollowersCount       int `json:"followers_count"`
			FollowingCount       int `json:"following_count"`
			PendingRequestsCount int `json:"pending_requests_count"`
		}

		err := db.QueryRow(`
			SELECT 
				(SELECT COUNT(*) FROM followers WHERE following_id = $1 AND status = 'accepted') as followers,
				(SELECT COUNT(*) FROM followers WHERE follower_id = $1 AND status = 'accepted') as following,
				(SELECT COUNT(*) FROM followers WHERE following_id = $1 AND status = 'pending') as pending`,
			userID).Scan(&stats.FollowersCount, &stats.FollowingCount, &stats.PendingRequestsCount)

		if err != nil {
			http.Error(w, "Failed to fetch follow stats", http.StatusInternalServerError)
			log.Println("GetFollowStats error:", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}

// CheckFollowStatus - Check follow status between two users
func CheckFollowStatus(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])
		targetUserID, _ := strconv.Atoi(vars["target_user_id"])

		var status struct {
			IsFollowing       bool   `json:"is_following"`        // Accepted follow
			IsFollower        bool   `json:"is_follower"`         // Accepted follower
			FollowRequestSent bool   `json:"follow_request_sent"` // Pending request sent
			FollowRequestFrom bool   `json:"follow_request_from"` // Pending request received
			FollowStatus      string `json:"follow_status"`       // Current status: "none", "pending", "accepted", "rejected"
		}

		var currentStatus sql.NullString
		err := db.QueryRow(`
			SELECT status FROM followers 
			WHERE follower_id = $1 AND following_id = $2`,
			userID, targetUserID).Scan(&currentStatus)

		if err == nil && currentStatus.Valid {
			status.FollowStatus = currentStatus.String
			status.IsFollowing = (currentStatus.String == "accepted")
			status.FollowRequestSent = (currentStatus.String == "pending")
		} else {
			status.FollowStatus = "none"
		}

		var reverseStatus sql.NullString
		err = db.QueryRow(`
			SELECT status FROM followers 
			WHERE follower_id = $1 AND following_id = $2`,
			targetUserID, userID).Scan(&reverseStatus)

		if err == nil && reverseStatus.Valid {
			status.IsFollower = (reverseStatus.String == "accepted")
			status.FollowRequestFrom = (reverseStatus.String == "pending")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	}
}

// Notification helpers
func notifyNewFollower(db *sql.DB, followerID, followingID int) {
	var followerName string
	err := db.QueryRow("SELECT display_name FROM users WHERE id = $1", followerID).Scan(&followerName)
	if err != nil {
		log.Printf("Error getting follower name: %v", err)
		followerName = "Someone"
	}

	rows, err := db.Query(`
		SELECT token FROM fcm_tokens 
		WHERE user_id = $1 AND token IS NOT NULL AND token != ''`,
		followingID)
	if err != nil {
		log.Printf("Error fetching FCM tokens: %v", err)
		return
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			continue
		}
		tokens = append(tokens, token)
	}

	if len(tokens) > 0 {
		data := map[string]string{
			"type":        "new_follower",
			"follower_id": strconv.Itoa(followerID),
		}
		services.SendMultipleNotifications(tokens, "New Follower",
			followerName+" started following you!", data)
	}
}

func notifyFollowRequest(db *sql.DB, followerID, followingID int) {
	var followerName string
	err := db.QueryRow("SELECT display_name FROM users WHERE id = $1", followerID).Scan(&followerName)
	if err != nil {
		log.Printf("Error getting follower name: %v", err)
		followerName = "Someone"
	}

	rows, err := db.Query(`
		SELECT token FROM fcm_tokens 
		WHERE user_id = $1 AND token IS NOT NULL AND token != ''`,
		followingID)
	if err != nil {
		log.Printf("Error fetching FCM tokens: %v", err)
		return
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			continue
		}
		tokens = append(tokens, token)
	}

	if len(tokens) > 0 {
		data := map[string]string{
			"type":        "follow_request",
			"follower_id": strconv.Itoa(followerID),
		}
		services.SendMultipleNotifications(tokens, "Follow Request",
			followerName+" wants to follow you", data)
	}
}

func notifyFollowAccepted(db *sql.DB, accepterID, followerID int) {
	var accepterName string
	err := db.QueryRow("SELECT display_name FROM users WHERE id = $1", accepterID).Scan(&accepterName)
	if err != nil {
		log.Printf("Error getting accepter name: %v", err)
		accepterName = "Someone"
	}

	rows, err := db.Query(`
		SELECT token FROM fcm_tokens 
		WHERE user_id = $1 AND token IS NOT NULL AND token != ''`,
		followerID)
	if err != nil {
		log.Printf("Error fetching FCM tokens: %v", err)
		return
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			continue
		}
		tokens = append(tokens, token)
	}

	if len(tokens) > 0 {
		data := map[string]string{
			"type":    "follow_accepted",
			"user_id": strconv.Itoa(accepterID),
		}
		services.SendMultipleNotifications(tokens, "Follow Request Accepted",
			accepterName+" accepted your follow request!", data)
	}
}
