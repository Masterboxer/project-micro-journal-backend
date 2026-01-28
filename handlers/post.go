package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
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
			       created_at,
			       journal_date
			FROM posts
			WHERE user_id = $1
			ORDER BY journal_date DESC, created_at DESC`,
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
				&p.JournalDate,
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

		var timezone string
		err = db.QueryRow(`SELECT timezone FROM users WHERE id = $1`, userID).Scan(&timezone)
		if err != nil {
			http.Error(w, "Failed to fetch user timezone", http.StatusInternalServerError)
			log.Println("GetUserFeed timezone error:", err)
			return
		}

		nowUTC := time.Now().UTC()
		todayJournalDate, err := ComputeJournalDate(nowUTC, timezone)
		if err != nil {
			http.Error(w, "Failed to compute journal date", http.StatusInternalServerError)
			log.Println("GetUserFeed journal date error:", err)
			return
		}

		startJournalDate := todayJournalDate.AddDate(0, 0, -1)

		rows, err := db.Query(`
			SELECT 
				p.id,
				p.user_id,
				p.template_id,
				p.text,
				COALESCE(p.photo_path, '') AS photo_path,
				p.created_at,
				p.journal_date,
				u.username,
				u.display_name,
				COALESCE((SELECT COUNT(*) FROM comments WHERE post_id = p.id), 0) AS comment_count,
				COALESCE((SELECT COUNT(*) FROM reactions WHERE post_id = p.id), 0) AS total_reactions,
				(SELECT reaction_type FROM reactions WHERE post_id = p.id AND user_id = $1) AS user_reaction
			FROM posts p
			JOIN users u ON p.user_id = u.id
			WHERE (
				p.user_id = $1
				OR p.user_id IN (
					SELECT following_id FROM followers 
					WHERE follower_id = $1 AND status = 'accepted'
					UNION
					SELECT follower_id FROM followers 
					WHERE following_id = $1 AND status = 'accepted'
				)
			)
			AND p.journal_date >= $2
			ORDER BY p.journal_date DESC, p.created_at DESC
		`, userID, startJournalDate)

		if err != nil {
			http.Error(w, "Failed to fetch feed", http.StatusInternalServerError)
			log.Printf("GetUserFeed query error: %v", err)
			return
		}
		defer rows.Close()

		var feed []map[string]interface{}
		for rows.Next() {
			var post struct {
				ID             int
				UserID         int
				TemplateID     int
				Text           string
				PhotoPath      string
				CreatedAt      time.Time
				JournalDate    time.Time
				Username       string
				DisplayName    string
				CommentCount   int
				TotalReactions int
				UserReaction   sql.NullString
			}

			if err := rows.Scan(
				&post.ID,
				&post.UserID,
				&post.TemplateID,
				&post.Text,
				&post.PhotoPath,
				&post.CreatedAt,
				&post.JournalDate,
				&post.Username,
				&post.DisplayName,
				&post.CommentCount,
				&post.TotalReactions,
				&post.UserReaction,
			); err != nil {
				http.Error(w, "Error scanning feed", http.StatusInternalServerError)
				log.Println("GetUserFeed scan error:", err)
				return
			}

			var userReaction interface{} = nil
			if post.UserReaction.Valid {
				userReaction = post.UserReaction.String
			}

			feed = append(feed, map[string]interface{}{
				"id":              post.ID,
				"user_id":         post.UserID,
				"template_id":     post.TemplateID,
				"text":            post.Text,
				"photo_path":      post.PhotoPath,
				"created_at":      post.CreatedAt.Format(time.RFC3339),
				"journal_date":    post.JournalDate.Format("2006-01-02"),
				"username":        post.Username,
				"display_name":    post.DisplayName,
				"comment_count":   post.CommentCount,
				"total_reactions": post.TotalReactions,
				"user_reaction":   userReaction,
			})
		}

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

		if p.UserID == 0 || p.TemplateID == 0 || p.Text == "" {
			http.Error(w, "Missing required fields", http.StatusBadRequest)
			return
		}

		if len(p.Text) > 280 {
			http.Error(w, "Text must be at most 280 characters", http.StatusBadRequest)
			return
		}

		var timezone string
		err := db.QueryRow(
			`SELECT timezone FROM users WHERE id = $1`,
			p.UserID,
		).Scan(&timezone)

		if err != nil {
			http.Error(w, "Failed to fetch user timezone", http.StatusInternalServerError)
			log.Println("CreatePost timezone error:", err)
			return
		}

		nowUTC := time.Now().UTC()
		journalDate, err := ComputeJournalDate(nowUTC, timezone)
		if err != nil {
			http.Error(w, "Failed to compute journal date", http.StatusInternalServerError)
			log.Println("CreatePost journal date error:", err)
			return
		}

		var existingPostID int
		var existingCreatedAt time.Time
		err = db.QueryRow(`
			SELECT id, created_at
			FROM posts
			WHERE user_id = $1 AND journal_date = $2
			LIMIT 1`,
			p.UserID, journalDate,
		).Scan(&existingPostID, &existingCreatedAt)

		if err != nil && err != sql.ErrNoRows {
			http.Error(w, "Failed to check existing posts", http.StatusInternalServerError)
			log.Println("CreatePost check existing error:", err)
			return
		}

		if err != sql.ErrNoRows {
			loc, _ := time.LoadLocation(timezone)
			localNow := nowUTC.In(loc)

			existingLocalTime := existingCreatedAt.In(loc)

			if existingLocalTime.Hour() < 12 && localNow.Hour() >= 12 &&
				existingLocalTime.Year() == localNow.Year() &&
				existingLocalTime.Month() == localNow.Month() &&
				existingLocalTime.Day() == localNow.Day() {
			} else {
				http.Error(w, "You already posted for this day", http.StatusForbidden)
				return
			}
		}

		err = db.QueryRow(`
			INSERT INTO posts (
				user_id,
				template_id,
				text,
				photo_path,
				journal_date,
				created_at
			)
			VALUES ($1, $2, $3, $4, $5, NOW())
			RETURNING id, created_at
		`,
			p.UserID,
			p.TemplateID,
			p.Text,
			p.PhotoPath,
			journalDate,
		).Scan(&p.ID, &p.CreatedAt)

		if err != nil {
			if strings.Contains(err.Error(), "uniq_user_journal_date") {
				http.Error(w, "You already posted for this day", http.StatusForbidden)
				return
			}

			http.Error(w, "Failed to create post", http.StatusInternalServerError)
			log.Println("CreatePost insert error:", err)
			return
		}

		go UpdateStreakAfterPost(db, p.UserID, journalDate)

		go notifyFollowersOfNewPost(db, p.UserID, p.Text)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(p)
	}
}

func ComputeJournalDate(now time.Time, timezone string) (time.Time, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}

	local := now.In(loc)

	cutoff := time.Date(
		local.Year(),
		local.Month(),
		local.Day(),
		12, 0, 0, 0,
		loc,
	)

	if local.Before(cutoff) {
		local = local.AddDate(0, 0, -1)
	}

	return time.Date(
		local.Year(),
		local.Month(),
		local.Day(),
		0, 0, 0, 0,
		loc,
	), nil
}

func notifyFollowersOfNewPost(db *sql.DB, userID int, postText string) {
	var displayName string
	err := db.QueryRow(`SELECT display_name FROM users WHERE id = $1`, userID).Scan(&displayName)
	if err != nil {
		log.Printf("Error fetching user display name for notifications: %v", err)
		displayName = "A friend"
	}

	rows, err := db.Query(`
		SELECT DISTINCT ft.token
		FROM followers f
		JOIN fcm_tokens ft ON f.follower_id = ft.user_id
		WHERE f.following_id = $1 
		  AND f.status = 'accepted'
		  AND ft.token IS NOT NULL 
		  AND ft.token != ''`,
		userID)
	if err != nil {
		log.Printf("Error fetching follower FCM tokens: %v", err)
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
		log.Printf("No FCM tokens found for user %d's followers", userID)
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

	successCount, failureCount, err := services.SendMultipleNotifications(db, tokens, title, body, data)
	if err != nil {
		log.Printf("Error sending notifications to followers: %v", err)
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

		var timezone string
		err = db.QueryRow(`SELECT timezone FROM users WHERE id = $1`, userID).Scan(&timezone)
		if err != nil {
			http.Error(w, "Failed to fetch user timezone", http.StatusInternalServerError)
			log.Printf("GetTodayPostForUser timezone error: %v", err)
			return
		}

		todayJournalDate, err := ComputeJournalDate(time.Now().UTC(), timezone)
		if err != nil {
			http.Error(w, "Failed to compute journal date", http.StatusInternalServerError)
			log.Printf("GetTodayPostForUser journal date error: %v", err)
			return
		}

		var p models.Post
		err = db.QueryRow(`
			SELECT id, user_id, template_id, text, photo_path, created_at, journal_date
			FROM posts
			WHERE user_id = $1
			  AND journal_date = $2
			ORDER BY created_at DESC
			LIMIT 1`,
			userID, todayJournalDate,
		).Scan(
			&p.ID,
			&p.UserID,
			&p.TemplateID,
			&p.Text,
			&p.PhotoPath,
			&p.CreatedAt,
			&p.JournalDate,
		)

		if err != nil {
			if err == sql.ErrNoRows {
				w.WriteHeader(http.StatusNoContent)
			} else {
				http.Error(w, "Database query failed", http.StatusInternalServerError)
				log.Printf("GetTodayPostForUser query error: %v", err)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	}
}

func AddReaction(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		postIDStr := vars["postId"]

		var req struct {
			UserID       int    `json:"user_id"`
			ReactionType string `json:"reaction_type"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		validReactions := map[string]bool{
			"heart": true, "laugh": true, "sad": true, "angry": true, "surprised": true,
		}
		if !validReactions[req.ReactionType] {
			http.Error(w, "Invalid reaction type", http.StatusBadRequest)
			return
		}

		postID, err := strconv.Atoi(postIDStr)
		if err != nil {
			http.Error(w, "Invalid post ID", http.StatusBadRequest)
			return
		}

		var existingReactionID int
		var existingReactionType string
		err = db.QueryRow(`
            SELECT id, reaction_type FROM reactions 
            WHERE user_id = $1 AND post_id = $2`,
			req.UserID, postID).Scan(&existingReactionID, &existingReactionType)

		if err == sql.ErrNoRows {
			var reactionID int
			err = db.QueryRow(`
                INSERT INTO reactions (user_id, post_id, reaction_type)
                VALUES ($1, $2, $3)
                RETURNING id`,
				req.UserID, postID, req.ReactionType).Scan(&reactionID)

			if err != nil {
				http.Error(w, "Failed to create reaction", http.StatusInternalServerError)
				log.Println("AddReaction create error:", err)
				return
			}

			go notifyPostOwnerOfReaction(db, postID, req.UserID, req.ReactionType)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"reaction_id":   reactionID,
				"reaction_type": req.ReactionType,
			})
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			log.Println("AddReaction query error:", err)
			return
		} else {
			if existingReactionType == req.ReactionType {
				_, err = db.Exec(`DELETE FROM reactions WHERE id = $1`, existingReactionID)
				if err != nil {
					http.Error(w, "Failed to remove reaction", http.StatusInternalServerError)
					log.Println("AddReaction delete error:", err)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"removed": true,
				})
			} else {
				_, err = db.Exec(`
                    UPDATE reactions 
                    SET reaction_type = $1, created_at = NOW() 
                    WHERE id = $2`,
					req.ReactionType, existingReactionID)

				if err != nil {
					http.Error(w, "Failed to update reaction", http.StatusInternalServerError)
					log.Println("AddReaction update error:", err)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"reaction_id":   existingReactionID,
					"reaction_type": req.ReactionType,
					"updated":       true,
				})
			}
		}
	}
}

func GetPostReactions(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		postID := vars["postId"]

		rows, err := db.Query(`
            SELECT r.id, r.post_id, r.user_id, r.reaction_type, r.created_at,
                   u.username, u.display_name
            FROM reactions r
            JOIN users u ON r.user_id = u.id
            WHERE r.post_id = $1
            ORDER BY r.created_at DESC`,
			postID)

		if err != nil {
			http.Error(w, "Failed to fetch reactions", http.StatusInternalServerError)
			log.Println("GetPostReactions error:", err)
			return
		}
		defer rows.Close()

		var reactions []map[string]interface{}
		for rows.Next() {
			var reaction struct {
				ID           int
				PostID       int
				UserID       int
				ReactionType string
				CreatedAt    string
				Username     string
				DisplayName  string
			}

			if err := rows.Scan(&reaction.ID, &reaction.PostID, &reaction.UserID,
				&reaction.ReactionType, &reaction.CreatedAt,
				&reaction.Username, &reaction.DisplayName); err != nil {
				http.Error(w, "Error scanning reactions", http.StatusInternalServerError)
				return
			}

			reactions = append(reactions, map[string]interface{}{
				"id":            reaction.ID,
				"post_id":       reaction.PostID,
				"user_id":       reaction.UserID,
				"reaction_type": reaction.ReactionType,
				"created_at":    reaction.CreatedAt,
				"username":      reaction.Username,
				"display_name":  reaction.DisplayName,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(reactions)
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

func notifyPostOwnerOfReaction(db *sql.DB, postID int, reactorUserID int, reactionType string) {
	var postOwnerID int
	var postText string
	var reactorDisplayName string

	err := db.QueryRow(`
		SELECT user_id, text 
		FROM posts 
		WHERE id = $1`, postID).Scan(&postOwnerID, &postText)

	if err != nil {
		log.Printf("Error fetching post info for reaction notification: %v", err)
		return
	}

	if postOwnerID == reactorUserID {
		return
	}

	err = db.QueryRow(`
		SELECT display_name 
		FROM users 
		WHERE id = $1`, reactorUserID).Scan(&reactorDisplayName)

	if err != nil {
		log.Printf("Error fetching reactor display name: %v", err)
		reactorDisplayName = "Someone"
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
		return
	}

	reactionEmojis := map[string]string{
		"heart":     "❤️",
		"laugh":     "😂",
		"sad":       "😢",
		"angry":     "😠",
		"surprised": "🤯",
	}

	emoji := reactionEmojis[reactionType]
	title := fmt.Sprintf("%s reacted %s to your post", reactorDisplayName, emoji)
	body := postText
	if len(body) > 100 {
		body = body[:97] + "..."
	}

	data := map[string]string{
		"type":          "post_reaction",
		"post_id":       strconv.Itoa(postID),
		"reactor_id":    strconv.Itoa(reactorUserID),
		"reaction_type": reactionType,
		"post_owner_id": strconv.Itoa(postOwnerID),
	}

	successCount, failureCount, err := services.SendMultipleNotifications(db, tokens, title, body, data)
	if err != nil {
		log.Printf("Error sending reaction notification: %v", err)
		return
	}

	log.Printf("Sent reaction notification for post %d: %d successful, %d failed",
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

	successCount, failureCount, err := services.SendMultipleNotifications(db, tokens, title, body, data)
	if err != nil {
		log.Printf("Error sending comment notification: %v", err)
		return
	}

	log.Printf("Sent comment notification for post %d: %d successful, %d failed",
		postID, successCount, failureCount)
}
