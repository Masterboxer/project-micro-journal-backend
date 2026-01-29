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
	log.Printf("🔥 UpdateStreakAfterPost called for user %d, journal_date: %s",
		userID, journalDate.Format("2006-01-02"))

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
		log.Printf("🔥 No existing streak, creating new one")

		_, err := db.Exec(`
			INSERT INTO streaks (
				user_id,
				streak_count,
				longest_streak,
				last_post_date,
				started_at,
				updated_at
			)
			VALUES ($1, 1, 1, $2, NOW(), NOW())
		`, userID, journalDate)

		if err != nil {
			log.Printf("❌ Failed to create streak: %v", err)
		} else {
			log.Printf("✅ Created new streak for user %d", userID)
		}
		return
	}

	if err != nil {
		log.Printf("❌ Failed to query streak: %v", err)
		return
	}

	log.Printf(
		"🔥 Current streak: count=%d, longest=%d, last_post_date=%s",
		currentCount,
		longestStreak,
		lastPostDate.String,
	)

	var lastDate *time.Time
	if lastPostDate.Valid {
		t, err := time.Parse("2006-01-02", lastPostDate.String[:10])
		if err != nil {
			log.Printf("❌ Failed to parse last_post_date: %v", err)
			return
		}
		lastDate = &t
		log.Printf("🔥 Parsed last_post_date: %s", lastDate.Format("2006-01-02"))
	}

	newCount := 1

	if lastDate != nil {
		yesterday := lastDate.AddDate(0, 0, 1)

		log.Printf("🔍 DEBUG: lastDate=%s, yesterday=%s, journalDate=%s",
			lastDate.Format("2006-01-02"),
			yesterday.Format("2006-01-02"),
			journalDate.Format("2006-01-02"))
		log.Printf("🔍 DEBUG: Equal check: %v, Before check: %v",
			journalDate.Equal(yesterday),
			journalDate.Before(*lastDate))

		switch {
		case journalDate.Equal(*lastDate):
			// Duplicate - ignore
			log.Printf("⚠️ Duplicate journal_date %s, streak already counted",
				journalDate.Format("2006-01-02"))
			return

		case journalDate.Before(*lastDate):
			// Out of order - posting for a date in the past
			log.Printf("⚠️ Out-of-order journal_date %s < last_post_date %s, ignoring",
				journalDate.Format("2006-01-02"),
				lastDate.Format("2006-01-02"))
			return

		case journalDate.Equal(yesterday):
			// Consecutive day - increment
			newCount = currentCount + 1
			log.Printf("✅ Consecutive day! Incrementing streak: %d → %d",
				currentCount, newCount)

		default:
			// Gap detected - reset streak
			newCount = 1
			log.Printf("⚠️ Gap detected. Resetting streak to 1 (last=%s, current=%s)",
				lastDate.Format("2006-01-02"),
				journalDate.Format("2006-01-02"))
		}
	}

	newLongestStreak := longestStreak
	if newCount > longestStreak {
		newLongestStreak = newCount
		log.Printf(
			"🔥 New longest streak! %d → %d",
			longestStreak,
			newLongestStreak,
		)
	}

	log.Printf(
		"🔥 Updating streak: count=%d, longest=%d, journal_date=%s",
		newCount,
		newLongestStreak,
		journalDate.Format("2006-01-02"),
	)

	result, err := db.Exec(`
		UPDATE streaks
		SET streak_count   = $1,
		    longest_streak = $2,
		    last_post_date = $3,
		    updated_at     = NOW()
		WHERE id = $4
	`,
		newCount,
		newLongestStreak,
		journalDate,
		streakID,
	)

	if err != nil {
		log.Printf("❌ Failed to update streak: %v", err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf(
		"✅ Updated streak for user %d: count=%d, longest=%d, journal_date=%s (rows affected: %d)",
		userID,
		newCount,
		newLongestStreak,
		journalDate.Format("2006-01-02"),
		rowsAffected,
	)
}
