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

		var targetExists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", req.FollowingID).Scan(&targetExists)
		if err != nil || !targetExists {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		var alreadyFollowing bool
		err = db.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM followers 
				WHERE follower_id = $1 AND following_id = $2
			)`, followerID, req.FollowingID).Scan(&alreadyFollowing)

		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if alreadyFollowing {
			http.Error(w, "Already following this user", http.StatusConflict)
			return
		}

		_, err = db.Exec(`
			INSERT INTO followers (follower_id, following_id, created_at)
			VALUES ($1, $2, NOW())`,
			followerID, req.FollowingID)

		if err != nil {
			http.Error(w, "Failed to follow user", http.StatusInternalServerError)
			log.Println("FollowUser error:", err)
			return
		}

		go notifyNewFollower(db, followerID, req.FollowingID)

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Successfully followed user",
		})
	}
}

func UnfollowUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		followerID, _ := strconv.Atoi(vars["user_id"])
		followingID, _ := strconv.Atoi(vars["following_id"])

		result, err := db.Exec(`
			DELETE FROM followers 
			WHERE follower_id = $1 AND following_id = $2`,
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

func RemoveFollower(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])
		followerID, _ := strconv.Atoi(vars["follower_id"])

		result, err := db.Exec(`
			DELETE FROM followers 
			WHERE follower_id = $1 AND following_id = $2`,
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
			WHERE follower_id = $1 AND following_id = $2`,
			userID, targetUserID)

		if err != nil {
			http.Error(w, "Failed to unfollow", http.StatusInternalServerError)
			log.Println("UnfollowAndRemove unfollow error:", err)
			return
		}

		_, err = tx.Exec(`
			DELETE FROM followers 
			WHERE follower_id = $1 AND following_id = $2`,
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

func GetUserFollowers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])

		rows, err := db.Query(`
			SELECT u.id, u.username, u.display_name, f.created_at
			FROM followers f
			JOIN users u ON f.follower_id = u.id
			WHERE f.following_id = $1
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

func GetUserFollowing(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])

		rows, err := db.Query(`
			SELECT u.id, u.username, u.display_name, f.created_at
			FROM followers f
			JOIN users u ON f.following_id = u.id
			WHERE f.follower_id = $1
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

func GetFollowStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])

		var stats struct {
			FollowersCount int `json:"followers_count"`
			FollowingCount int `json:"following_count"`
		}

		err := db.QueryRow(`
			SELECT 
				(SELECT COUNT(*) FROM followers WHERE following_id = $1) as followers,
				(SELECT COUNT(*) FROM followers WHERE follower_id = $1) as following`,
			userID).Scan(&stats.FollowersCount, &stats.FollowingCount)

		if err != nil {
			http.Error(w, "Failed to fetch follow stats", http.StatusInternalServerError)
			log.Println("GetFollowStats error:", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}

func CheckFollowStatus(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])
		targetUserID, _ := strconv.Atoi(vars["target_user_id"])

		var status struct {
			IsFollowing bool `json:"is_following"`
			IsFollower  bool `json:"is_follower"`
		}

		err := db.QueryRow(`
			SELECT 
				EXISTS(SELECT 1 FROM followers WHERE follower_id = $1 AND following_id = $2) as following,
				EXISTS(SELECT 1 FROM followers WHERE follower_id = $2 AND following_id = $1) as follower`,
			userID, targetUserID).Scan(&status.IsFollowing, &status.IsFollower)

		if err != nil {
			http.Error(w, "Failed to check follow status", http.StatusInternalServerError)
			log.Println("CheckFollowStatus error:", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	}
}

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
