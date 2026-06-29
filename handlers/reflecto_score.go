package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

const (
	ScorePost     = 5
	ScoreComment  = 2
	ScoreLike     = 1
	ScoreReaction = 1
	ScoreDecay    = -1
)

type ActionType string

const (
	ActionPost     ActionType = "post"
	ActionComment  ActionType = "comment"
	ActionLike     ActionType = "like"
	ActionReaction ActionType = "reaction"
)

type ReflectoScore struct {
	ID           int       `json:"id"`
	UserID       int       `json:"user_id"`
	Score        int       `json:"score"`
	LastPostDate *string   `json:"last_post_date,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func GetUserReflectoScore(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, err := strconv.Atoi(vars["userId"])
		if err != nil {
			http.Error(w, "Invalid user ID", http.StatusBadRequest)
			return
		}

		var s ReflectoScore
		var lastPostDate sql.NullString

		err = db.QueryRow(`
			SELECT id, user_id, score, last_post_date, updated_at
			FROM reflecto_scores
			WHERE user_id = $1
		`, userID).Scan(&s.ID, &s.UserID, &s.Score, &lastPostDate, &s.UpdatedAt)

		if err == sql.ErrNoRows {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"score":  0,
				"exists": false,
			})
			return
		} else if err != nil {
			log.Printf("GetUserReflectoScore error: %v", err)
			http.Error(w, "Failed to fetch score", http.StatusInternalServerError)
			return
		}

		if lastPostDate.Valid {
			s.LastPostDate = &lastPostDate.String
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s)
	}
}

func SubtractReflectoScore(db *sql.DB, userID int, action ActionType, postID *int) {
	if action != ActionPost && postID != nil {
		_, err := db.Exec(`
            DELETE FROM reflecto_score_events
            WHERE user_id = $1 AND post_id = $2 AND action_type = $3
        `, userID, *postID, string(action))
		if err != nil {
			log.Printf("❌ SubtractReflectoScore event delete error: %v", err)
		}
	}

	points := pointsForAction(action)
	if points == 0 {
		return
	}

	log.Printf("📉 SubtractReflectoScore: user=%d action=%s points=-%d", userID, action, points)

	_, err := db.Exec(`
        UPDATE reflecto_scores
        SET score      = GREATEST(0, score - $1),
            updated_at = NOW()
        WHERE user_id  = $2
    `, points, userID)
	if err != nil {
		log.Printf("❌ SubtractReflectoScore error: %v", err)
	} else {
		log.Printf("✅ Score decremented for user %d (-%d for deleting %s)", userID, points, action)
	}
}

func AddReflectoScore(db *sql.DB, userID int, action ActionType, postDate *time.Time, postID *int) {
	points := pointsForAction(action)
	if points == 0 {
		log.Printf("⚠️ Unknown action type: %s", action)
		return
	}

	if action != ActionPost && postID != nil {
		result, err := db.Exec(`
            INSERT INTO reflecto_score_events (user_id, post_id, action_type)
            VALUES ($1, $2, $3)
            ON CONFLICT (user_id, post_id, action_type) DO NOTHING
        `, userID, *postID, string(action))
		if err != nil {
			log.Printf("❌ AddReflectoScore event insert error: %v", err)
			return
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			log.Printf("⏭️ Score already awarded for user=%d post=%d action=%s — skipping", userID, *postID, action)
			return
		}
	}

	log.Printf("🌟 AddReflectoScore: user=%d action=%s points=%d", userID, action, points)

	if action == ActionPost && postDate != nil {
		dateStr := postDate.UTC().Format("2006-01-02")
		_, err := db.Exec(`
            INSERT INTO reflecto_scores (user_id, score, last_post_date, updated_at)
            VALUES ($1, $2, $3, NOW())
            ON CONFLICT (user_id) DO UPDATE
            SET score          = GREATEST(0, reflecto_scores.score + $2),
                last_post_date = CASE
                    WHEN $3::date > reflecto_scores.last_post_date OR reflecto_scores.last_post_date IS NULL
                    THEN $3::date
                    ELSE reflecto_scores.last_post_date
                END,
                updated_at     = NOW()
        `, userID, points, dateStr)
		if err != nil {
			log.Printf("❌ AddReflectoScore (post) error: %v", err)
		} else {
			log.Printf("✅ Score updated for user %d (+%d for %s)", userID, points, action)
		}
		return
	}

	_, err := db.Exec(`
        INSERT INTO reflecto_scores (user_id, score, updated_at)
        VALUES ($1, $2, NOW())
        ON CONFLICT (user_id) DO UPDATE
        SET score      = GREATEST(0, reflecto_scores.score + $2),
            updated_at = NOW()
    `, userID, points)
	if err != nil {
		log.Printf("❌ AddReflectoScore error: %v", err)
	} else {
		log.Printf("✅ Score updated for user %d (+%d for %s)", userID, points, action)
	}
}

func ApplyDailyDecay(db *sql.DB) {
	today := time.Now().UTC().Format("2006-01-02")
	log.Printf("📉 ApplyDailyDecay running for date: %s", today)

	result, err := db.Exec(`
		UPDATE reflecto_scores
		SET score      = GREATEST(0, score + $1),
		    updated_at = NOW()
		WHERE last_post_date IS NULL
		   OR last_post_date < $2::date
	`, ScoreDecay, today)

	if err != nil {
		log.Printf("❌ ApplyDailyDecay error: %v", err)
		return
	}

	rows, _ := result.RowsAffected()
	log.Printf("✅ ApplyDailyDecay: decayed %d users by %d point(s)", rows, -ScoreDecay)
}

func pointsForAction(action ActionType) int {
	switch action {
	case ActionPost:
		return ScorePost
	case ActionComment:
		return ScoreComment
	case ActionLike:
		return ScoreLike
	case ActionReaction:
		return ScoreReaction
	default:
		return 0
	}
}
