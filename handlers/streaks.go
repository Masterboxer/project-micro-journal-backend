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

type Streak struct {
	ID            int       `json:"id"`
	UserID        int       `json:"user_id"`
	StreakCount   int       `json:"streak_count"`
	LastPostDate  *string   `json:"last_post_date,omitempty"`
	LongestStreak int       `json:"longest_streak"`
	StartedAt     time.Time `json:"started_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func GetUserStreak(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userIDStr := vars["userId"]
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			http.Error(w, "Invalid user ID", http.StatusBadRequest)
			return
		}

		var s Streak
		var lastPostDate sql.NullString
		err = db.QueryRow(`
			SELECT 
				id,
				user_id,
				streak_count,
				last_post_date,
				longest_streak,
				started_at,
				updated_at
			FROM streaks
			WHERE user_id = $1
		`, userID).Scan(
			&s.ID,
			&s.UserID,
			&s.StreakCount,
			&lastPostDate,
			&s.LongestStreak,
			&s.StartedAt,
			&s.UpdatedAt,
		)

		if err == sql.ErrNoRows {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"streak_count":   0,
				"longest_streak": 0,
				"exists":         false,
			})
			return
		} else if err != nil {
			http.Error(w, "Failed to fetch streak", http.StatusInternalServerError)
			log.Printf("GetUserStreak error: %v", err)
			return
		}

		if lastPostDate.Valid {
			s.LastPostDate = &lastPostDate.String
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s)
	}
}

func UpdateStreakAfterPost(db *sql.DB, userID int, journalDate time.Time) {
	var streakID int
	var currentCount int
	var longestStreak int
	var lastPostDate sql.NullString

	err := db.QueryRow(`
		SELECT id, streak_count, longest_streak, last_post_date
		FROM streaks
		WHERE user_id = $1
	`, userID).Scan(&streakID, &currentCount, &longestStreak, &lastPostDate)

	if err == sql.ErrNoRows {
		_, err = db.Exec(`
			INSERT INTO streaks (user_id, streak_count, longest_streak, last_post_date)
			VALUES ($1, 1, 1, $2)
		`, userID, journalDate)
		if err != nil {
			log.Printf("Failed to create streak: %v", err)
		}
		return
	} else if err != nil {
		log.Printf("Failed to query streak: %v", err)
		return
	}

	var lastDate *time.Time
	if lastPostDate.Valid {
		t, err := time.Parse("2006-01-02", lastPostDate.String)
		if err != nil {
			log.Printf("Failed to parse last_post_date: %v", err)
			return
		}
		lastDate = &t
	}

	if lastDate != nil && lastDate.Equal(journalDate) {
		log.Printf("Post for journal_date %s already counted in streak", journalDate.Format("2006-01-02"))
		return
	}

	newCount := 1
	yesterday := journalDate.AddDate(0, 0, -1)

	if lastDate != nil {
		if lastDate.Equal(yesterday) {
			newCount = currentCount + 1
		} else if lastDate.After(yesterday) {
			newCount = currentCount
			log.Printf("Warning: Posting for date %s which is before or equal to last_post_date %s",
				journalDate.Format("2006-01-02"), lastDate.Format("2006-01-02"))
		}
	}

	newLongestStreak := longestStreak
	if newCount > longestStreak {
		newLongestStreak = newCount
	}

	_, err = db.Exec(`
		UPDATE streaks
		SET streak_count = $1, 
		    longest_streak = $2,
		    last_post_date = $3,
		    updated_at = NOW()
		WHERE id = $4
	`, newCount, newLongestStreak, journalDate, streakID)

	if err != nil {
		log.Printf("Failed to update streak: %v", err)
		return
	}

	log.Printf("Updated streak for user %d: count=%d, longest=%d, journal_date=%s",
		userID, newCount, newLongestStreak, journalDate.Format("2006-01-02"))
}
