package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"masterboxer.com/project-micro-journal/models"
	"masterboxer.com/project-micro-journal/services"
)

func GetPostsByUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userIDStr, ok := vars["userId"]
		if !ok || userIDStr == "" {
			http.Error(w, "userId parameter missing", http.StatusBadRequest)
			return
		}

		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			http.Error(w, "Invalid userId", http.StatusBadRequest)
			return
		}

		rows, err := db.Query(`
			SELECT id, user_id, template_id, text, 
			       COALESCE(photo_path, '') as photo_path, 
			       created_at
			FROM posts
			WHERE user_id = $1
			ORDER BY created_at DESC`,
			userID)
		if err != nil {
			http.Error(w, "Database query failed", http.StatusInternalServerError)
			log.Printf("GetPostsByUser error: %v", err)
			return
		}
		defer rows.Close()

		var posts []models.Post
		for rows.Next() {
			var p models.Post
			if err := rows.Scan(
				&p.ID,
				&p.UserID,
				&p.TemplateID,
				&p.Text,
				&p.PhotoPath,
				&p.CreatedAt,
			); err != nil {
				http.Error(w, "Error scanning posts", http.StatusInternalServerError)
				log.Printf("GetPostsByUser scan error: %v", err)
				return
			}
			posts = append(posts, p)
		}

		if err := rows.Err(); err != nil {
			http.Error(w, "Error iterating posts", http.StatusInternalServerError)
			log.Printf("GetPostsByUser rows error: %v", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(posts)
	}
}

func GetUserFeed(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userIDStr, ok := vars["userId"]
		if !ok || userIDStr == "" {
			http.Error(w, "userId parameter missing", http.StatusBadRequest)
			return
		}

		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			http.Error(w, "Invalid userId", http.StatusBadRequest)
			return
		}

		log.Printf("GetUserFeed called for user ID: %d", userID)

		thirtySixHoursAgo := time.Now().Add(-36 * time.Hour)

		rows, err := db.Query(`
            SELECT p.id, p.user_id, p.template_id, p.text, 
                   COALESCE(p.photo_path, '') as photo_path, 
                   p.created_at, 
                   u.username, u.display_name,
                   COALESCE((SELECT COUNT(*) FROM likes WHERE post_id = p.id), 0) as like_count,
                   COALESCE((SELECT COUNT(*) FROM comments WHERE post_id = p.id), 0) as comment_count,
                   EXISTS(SELECT 1 FROM likes WHERE post_id = p.id AND user_id = $1) as is_liked_by_user
            FROM posts p
            JOIN users u ON p.user_id = u.id
            WHERE (p.user_id = $1 
               OR p.user_id IN (
                   SELECT buddy_id FROM buddies WHERE user_id = $1
                   UNION
                   SELECT user_id FROM buddies WHERE buddy_id = $1
               ))
            AND p.created_at >= $2
            ORDER BY p.created_at DESC`,
			userID, thirtySixHoursAgo)

		if err != nil {
			http.Error(w, "Failed to fetch feed", http.StatusInternalServerError)
			log.Printf("GetUserFeed query error: %v", err)
			return
		}
		defer rows.Close()

		var feed []map[string]interface{}
		for rows.Next() {
			var post struct {
				ID           int
				UserID       int
				TemplateID   int
				Text         string
				PhotoPath    string
				CreatedAt    time.Time
				Username     string
				DisplayName  string
				LikeCount    int
				CommentCount int
				IsLiked      bool
			}

			if err := rows.Scan(&post.ID, &post.UserID, &post.TemplateID,
				&post.Text, &post.PhotoPath, &post.CreatedAt,
				&post.Username, &post.DisplayName, &post.LikeCount,
				&post.CommentCount, &post.IsLiked); err != nil {
				http.Error(w, "Error scanning posts", http.StatusInternalServerError)
				log.Printf("GetUserFeed scan error: %v", err)
				return
			}

			feed = append(feed, map[string]interface{}{
				"id":               post.ID,
				"user_id":          post.UserID,
				"template_id":      post.TemplateID,
				"text":             post.Text,
				"photo_path":       post.PhotoPath,
				"created_at":       post.CreatedAt.Format(time.RFC3339),
				"username":         post.Username,
				"display_name":     post.DisplayName,
				"like_count":       post.LikeCount,
				"comment_count":    post.CommentCount,
				"is_liked_by_user": post.IsLiked,
			})
		}

		log.Printf("GetUserFeed returning %d posts for user %d (from last 36 hours)", len(feed), userID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(feed)
	}
}

func CreatePost(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p models.Post
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if p.UserID == 0 {
			http.Error(w, "user_id is required", http.StatusBadRequest)
			return
		}
		if p.TemplateID == 0 {
			http.Error(w, "template_id is required", http.StatusBadRequest)
			return
		}
		if p.Text == "" {
			http.Error(w, "text is required", http.StatusBadRequest)
			return
		}
		if len(p.Text) > 280 {
			http.Error(w, "text must be at most 280 characters", http.StatusBadRequest)
			return
		}

		now := time.Now().UTC()
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		endOfDay := startOfDay.Add(24 * time.Hour)

		var postCount int
		err := db.QueryRow(`
			SELECT COUNT(*) 
			FROM posts 
			WHERE user_id = $1 
			  AND created_at >= $2 
			  AND created_at < $3`,
			p.UserID, startOfDay, endOfDay).Scan(&postCount)
		if err != nil {
			http.Error(w, "Failed to check daily limit", http.StatusInternalServerError)
			log.Println("CreatePost daily limit check error:", err)
			return
		}

		if postCount > 0 {
			http.Error(w, "Daily post limit reached (1 post per day)", http.StatusForbidden)
			return
		}

		err = db.QueryRow(`
			INSERT INTO posts (user_id, template_id, text, photo_path, created_at)
			VALUES ($1, $2, $3, $4, NOW())
			RETURNING id, user_id, template_id, text, photo_path, created_at`,
			p.UserID,
			p.TemplateID,
			p.Text,
			p.PhotoPath,
		).Scan(
			&p.ID,
			&p.UserID,
			&p.TemplateID,
			&p.Text,
			&p.PhotoPath,
			&p.CreatedAt,
		)
		if err != nil {
			http.Error(w, "Failed to create post", http.StatusInternalServerError)
			log.Println("CreatePost error:", err)
			return
		}

		go notifyBuddiesOfNewPost(db, p.UserID, p.Text)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(p)
	}
}

func notifyBuddiesOfNewPost(db *sql.DB, userID int, postText string) {
	var displayName string
	err := db.QueryRow(`SELECT display_name FROM users WHERE id = $1`, userID).Scan(&displayName)
	if err != nil {
		log.Printf("Error fetching user display name for notifications: %v", err)
		displayName = "A friend"
	}

	rows, err := db.Query(`
		SELECT DISTINCT ft.token
		FROM buddies b
		JOIN fcm_tokens ft ON b.buddy_id = ft.user_id
		WHERE b.user_id = $1 
		  AND ft.token IS NOT NULL 
		  AND ft.token != ''`,
		userID)
	if err != nil {
		log.Printf("Error fetching buddy FCM tokens: %v", err)
		return
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			log.Printf("Error scanning FCM token: %v", err)
			continue
		}
		tokens = append(tokens, token)
	}

	if len(tokens) == 0 {
		log.Printf("No FCM tokens found for user %d's buddies", userID)
		return
	}

	title := fmt.Sprintf("%s posted today!", displayName)
	body := postText
	if len(body) > 100 {
		body = body[:97] + "..."
	}

	data := map[string]string{
		"type":    "new_post",
		"user_id": strconv.Itoa(userID),
	}

	successCount, failureCount, err := services.SendMultipleNotifications(tokens, title, body, data)
	if err != nil {
		log.Printf("Error sending notifications to buddies: %v", err)
		return
	}

	log.Printf("Sent notifications for new post by user %d: %d successful, %d failed",
		userID, successCount, failureCount)
}

func DeletePost(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]

		var exists bool
		err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM posts WHERE id = $1)`, id).
			Scan(&exists)
		if err != nil {
			http.Error(w, "Database query failed", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		if !exists {
			http.Error(w, "Post not found", http.StatusNotFound)
			return
		}

		_, err = db.Exec(`DELETE FROM posts WHERE id = $1`, id)
		if err != nil {
			http.Error(w, "Failed to delete post", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Post deleted successfully",
		})
	}
}

func GetTodayPostForUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var userID int
		var err error

		vars := mux.Vars(r)
		if uidStr, ok := vars["userId"]; ok {
			userID, err = strconv.Atoi(uidStr)
			if err != nil {
				http.Error(w, "Invalid userId", http.StatusBadRequest)
				return
			}
		} else {
			uidStr := r.URL.Query().Get("user_id")
			if uidStr == "" {
				http.Error(w, "user_id is required", http.StatusBadRequest)
				return
			}
			userID, err = strconv.Atoi(uidStr)
			if err != nil {
				http.Error(w, "Invalid user_id", http.StatusBadRequest)
				return
			}
		}

		now := time.Now()
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		endOfDay := startOfDay.Add(24 * time.Hour)

		var p models.Post
		err = db.QueryRow(`
			SELECT id, user_id, template_id, text, photo_path, created_at
			FROM posts
			WHERE user_id = $1
			  AND created_at >= $2
			  AND created_at < $3
			ORDER BY created_at DESC
			LIMIT 1`,
			userID, startOfDay, endOfDay,
		).Scan(
			&p.ID,
			&p.UserID,
			&p.TemplateID,
			&p.Text,
			&p.PhotoPath,
			&p.CreatedAt,
		)

		if err != nil {
			if err == sql.ErrNoRows {
				w.WriteHeader(http.StatusNoContent)
			} else {
				http.Error(w, "Database query failed", http.StatusInternalServerError)
				log.Println(err)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	}
}

func ToggleLike(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		postIDStr := vars["postId"]

		var req struct {
			UserID int `json:"user_id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		postID, err := strconv.Atoi(postIDStr)
		if err != nil {
			http.Error(w, "Invalid post ID", http.StatusBadRequest)
			return
		}

		var existingLikeID int
		err = db.QueryRow(`
            SELECT id FROM likes WHERE user_id = $1 AND post_id = $2`,
			req.UserID, postID).Scan(&existingLikeID)

		if err == sql.ErrNoRows {
			var likeID int
			err = db.QueryRow(`
                INSERT INTO likes (user_id, post_id)
                VALUES ($1, $2)
                RETURNING id`,
				req.UserID, postID).Scan(&likeID)

			if err != nil {
				http.Error(w, "Failed to create like", http.StatusInternalServerError)
				log.Println("ToggleLike create error:", err)
				return
			}

			go notifyPostOwnerOfLike(db, postID, req.UserID)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"liked":   true,
				"like_id": likeID,
			})
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			log.Println("ToggleLike query error:", err)
			return
		} else {
			_, err = db.Exec(`DELETE FROM likes WHERE id = $1`, existingLikeID)
			if err != nil {
				http.Error(w, "Failed to remove like", http.StatusInternalServerError)
				log.Println("ToggleLike delete error:", err)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"liked": false,
			})
		}
	}
}

func GetPostLikes(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		postID := vars["postId"]

		rows, err := db.Query(`
            SELECT l.id, l.post_id, l.user_id, l.created_at,
                   u.username, u.display_name
            FROM likes l
            JOIN users u ON l.user_id = u.id
            WHERE l.post_id = $1
            ORDER BY l.created_at DESC`,
			postID)

		if err != nil {
			http.Error(w, "Failed to fetch likes", http.StatusInternalServerError)
			log.Println("GetPostLikes error:", err)
			return
		}
		defer rows.Close()

		var likes []map[string]interface{}
		for rows.Next() {
			var like struct {
				ID          int
				PostID      int
				UserID      int
				CreatedAt   string
				Username    string
				DisplayName string
			}

			if err := rows.Scan(&like.ID, &like.PostID, &like.UserID,
				&like.CreatedAt, &like.Username, &like.DisplayName); err != nil {
				http.Error(w, "Error scanning likes", http.StatusInternalServerError)
				return
			}

			likes = append(likes, map[string]interface{}{
				"id":           like.ID,
				"post_id":      like.PostID,
				"user_id":      like.UserID,
				"created_at":   like.CreatedAt,
				"username":     like.Username,
				"display_name": like.DisplayName,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(likes)
	}
}

func CreateComment(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		postID := vars["postId"]

		var comment models.Comment
		if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if comment.Text == "" {
			http.Error(w, "Comment text is required", http.StatusBadRequest)
			return
		}

		if len(comment.Text) > 500 {
			http.Error(w, "Comment must be at most 500 characters", http.StatusBadRequest)
			return
		}

		postIDInt, err := strconv.Atoi(postID)
		if err != nil {
			http.Error(w, "Invalid post ID", http.StatusBadRequest)
			return
		}

		err = db.QueryRow(`
            INSERT INTO comments (post_id, user_id, text)
            VALUES ($1, $2, $3)
            RETURNING id, post_id, user_id, text, created_at`,
			postIDInt, comment.UserID, comment.Text,
		).Scan(&comment.ID, &comment.PostID, &comment.UserID,
			&comment.Text, &comment.CreatedAt)

		if err != nil {
			http.Error(w, "Failed to create comment", http.StatusInternalServerError)
			log.Println("CreateComment error:", err)
			return
		}

		// Send notification to post owner
		go notifyPostOwnerOfComment(db, postIDInt, comment.UserID, comment.Text)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(comment)
	}
}

func GetPostComments(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		postID := vars["postId"]

		rows, err := db.Query(`
            SELECT c.id, c.post_id, c.user_id, c.text, c.created_at,
                   u.username, u.display_name
            FROM comments c
            JOIN users u ON c.user_id = u.id
            WHERE c.post_id = $1
            ORDER BY c.created_at ASC`,
			postID)

		if err != nil {
			http.Error(w, "Failed to fetch comments", http.StatusInternalServerError)
			log.Println("GetPostComments error:", err)
			return
		}
		defer rows.Close()

		var comments []models.CommentWithUser
		for rows.Next() {
			var c models.CommentWithUser
			if err := rows.Scan(&c.ID, &c.PostID, &c.UserID, &c.Text,
				&c.CreatedAt, &c.Username, &c.DisplayName); err != nil {
				http.Error(w, "Error scanning comments", http.StatusInternalServerError)
				log.Println("GetPostComments scan error:", err)
				return
			}
			comments = append(comments, c)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(comments)
	}
}

func DeleteComment(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		commentID := vars["commentId"]

		var req struct {
			UserID int `json:"user_id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		var ownerID int
		err := db.QueryRow(`SELECT user_id FROM comments WHERE id = $1`,
			commentID).Scan(&ownerID)

		if err == sql.ErrNoRows {
			http.Error(w, "Comment not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if ownerID != req.UserID {
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}

		_, err = db.Exec(`DELETE FROM comments WHERE id = $1`, commentID)
		if err != nil {
			http.Error(w, "Failed to delete comment", http.StatusInternalServerError)
			log.Println("DeleteComment error:", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Comment deleted successfully",
		})
	}
}

func notifyPostOwnerOfLike(db *sql.DB, postID int, likerUserID int) {
	var postOwnerID int
	var postText string
	var likerDisplayName string

	err := db.QueryRow(`
		SELECT user_id, text 
		FROM posts 
		WHERE id = $1`, postID).Scan(&postOwnerID, &postText)

	if err != nil {
		log.Printf("Error fetching post info for like notification: %v", err)
		return
	}

	if postOwnerID == likerUserID {
		return
	}

	err = db.QueryRow(`
		SELECT display_name 
		FROM users 
		WHERE id = $1`, likerUserID).Scan(&likerDisplayName)

	if err != nil {
		log.Printf("Error fetching liker display name: %v", err)
		likerDisplayName = "Someone"
	}

	rows, err := db.Query(`
		SELECT token 
		FROM fcm_tokens 
		WHERE user_id = $1 
		  AND token IS NOT NULL 
		  AND token != ''`,
		postOwnerID)

	if err != nil {
		log.Printf("Error fetching post owner FCM tokens: %v", err)
		return
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			log.Printf("Error scanning FCM token: %v", err)
			continue
		}
		tokens = append(tokens, token)
	}

	if len(tokens) == 0 {
		log.Printf("No FCM tokens found for post owner %d", postOwnerID)
		return
	}

	title := fmt.Sprintf("%s liked your post", likerDisplayName)
	body := postText
	if len(body) > 100 {
		body = body[:97] + "..."
	}

	data := map[string]string{
		"type":          "post_like",
		"post_id":       strconv.Itoa(postID),
		"liker_id":      strconv.Itoa(likerUserID),
		"post_owner_id": strconv.Itoa(postOwnerID),
	}

	successCount, failureCount, err := services.SendMultipleNotifications(tokens, title, body, data)
	if err != nil {
		log.Printf("Error sending like notification: %v", err)
		return
	}

	log.Printf("Sent like notification for post %d: %d successful, %d failed",
		postID, successCount, failureCount)
}

func notifyPostOwnerOfComment(db *sql.DB, postID int, commenterUserID int, commentText string) {
	var postOwnerID int
	var postText string
	var commenterDisplayName string

	err := db.QueryRow(`
		SELECT user_id, text 
		FROM posts 
		WHERE id = $1`, postID).Scan(&postOwnerID, &postText)

	if err != nil {
		log.Printf("Error fetching post info for comment notification: %v", err)
		return
	}

	if postOwnerID == commenterUserID {
		return
	}

	err = db.QueryRow(`
		SELECT display_name 
		FROM users 
		WHERE id = $1`, commenterUserID).Scan(&commenterDisplayName)

	if err != nil {
		log.Printf("Error fetching commenter display name: %v", err)
		commenterDisplayName = "Someone"
	}

	rows, err := db.Query(`
		SELECT token 
		FROM fcm_tokens 
		WHERE user_id = $1 
		  AND token IS NOT NULL 
		  AND token != ''`,
		postOwnerID)

	if err != nil {
		log.Printf("Error fetching post owner FCM tokens: %v", err)
		return
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			log.Printf("Error scanning FCM token: %v", err)
			continue
		}
		tokens = append(tokens, token)
	}

	if len(tokens) == 0 {
		log.Printf("No FCM tokens found for post owner %d", postOwnerID)
		return
	}

	title := fmt.Sprintf("%s commented on your post", commenterDisplayName)
	body := commentText
	if len(body) > 100 {
		body = body[:97] + "..."
	}

	data := map[string]string{
		"type":          "post_comment",
		"post_id":       strconv.Itoa(postID),
		"commenter_id":  strconv.Itoa(commenterUserID),
		"post_owner_id": strconv.Itoa(postOwnerID),
		"comment_text":  commentText,
	}

	successCount, failureCount, err := services.SendMultipleNotifications(tokens, title, body, data)
	if err != nil {
		log.Printf("Error sending comment notification: %v", err)
		return
	}

	log.Printf("Sent comment notification for post %d: %d successful, %d failed",
		postID, successCount, failureCount)
}
