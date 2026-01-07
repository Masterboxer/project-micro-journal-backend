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

// Streak represents a posting streak between two users
type Streak struct {
	ID                    int       `json:"id"`
	UserID1               int       `json:"user_id_1"`
	UserID2               int       `json:"user_id_2"`
	StreakCount           int       `json:"streak_count"`
	LastContributionUser1 *string   `json:"last_contribution_date_user1,omitempty"`
	LastContributionUser2 *string   `json:"last_contribution_date_user2,omitempty"`
	StartedAt             time.Time `json:"started_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// StreakWithUser includes user details for display
type StreakWithUser struct {
	StreakID              int       `json:"streak_id"`
	OtherUserID           int       `json:"other_user_id"`
	OtherUsername         string    `json:"other_username"`
	OtherDisplayName      string    `json:"other_display_name"`
	StreakCount           int       `json:"streak_count"`
	LastContributionSelf  *string   `json:"last_contribution_self,omitempty"`
	LastContributionOther *string   `json:"last_contribution_other,omitempty"`
	StartedAt             time.Time `json:"started_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	IsActive              bool      `json:"is_active"`
	NeedsSelfPost         bool      `json:"needs_self_post"`
	NeedsOtherPost        bool      `json:"needs_other_post"`
}

// GetUserStreaks returns all streaks for a specific user
// GET /users/:userId/streaks
func GetUserStreaks(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userIDStr := vars["userId"]
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			http.Error(w, "Invalid user ID", http.StatusBadRequest)
			return
		}

		// Fetch user's timezone for journal_date computation
		var timezone string
		err = db.QueryRow(`SELECT timezone FROM users WHERE id = $1`, userID).Scan(&timezone)
		if err != nil {
			http.Error(w, "Failed to fetch user timezone", http.StatusInternalServerError)
			log.Printf("GetUserStreaks timezone error: %v", err)
			return
		}

		todayJournalDate, err := ComputeJournalDate(time.Now().UTC(), timezone)
		if err != nil {
			http.Error(w, "Failed to compute journal date", http.StatusInternalServerError)
			return
		}

		// Query streaks where user is either user_id_1 or user_id_2
		rows, err := db.Query(`
			SELECT 
				s.id,
				s.user_id_1,
				s.user_id_2,
				s.streak_count,
				s.last_contribution_date_user1,
				s.last_contribution_date_user2,
				s.started_at,
				s.updated_at,
				u.username,
				u.display_name
			FROM streaks s
			JOIN users u ON (
				CASE 
					WHEN s.user_id_1 = $1 THEN u.id = s.user_id_2
					WHEN s.user_id_2 = $1 THEN u.id = s.user_id_1
				END
			)
			WHERE s.user_id_1 = $1 OR s.user_id_2 = $1
			ORDER BY s.streak_count DESC, s.updated_at DESC
		`, userID)

		if err != nil {
			http.Error(w, "Failed to fetch streaks", http.StatusInternalServerError)
			log.Printf("GetUserStreaks query error: %v", err)
			return
		}
		defer rows.Close()

		var streaks []StreakWithUser
		for rows.Next() {
			var s Streak
			var otherUsername, otherDisplayName string
			var lastContrib1, lastContrib2 sql.NullString

			if err := rows.Scan(
				&s.ID,
				&s.UserID1,
				&s.UserID2,
				&s.StreakCount,
				&lastContrib1,
				&lastContrib2,
				&s.StartedAt,
				&s.UpdatedAt,
				&otherUsername,
				&otherDisplayName,
			); err != nil {
				http.Error(w, "Error scanning streaks", http.StatusInternalServerError)
				log.Printf("GetUserStreaks scan error: %v", err)
				return
			}

			// Determine other user and contribution dates
			var otherUserID int
			var lastContribSelf, lastContribOther *string

			if s.UserID1 == userID {
				otherUserID = s.UserID2
				if lastContrib1.Valid {
					lastContribSelf = &lastContrib1.String
				}
				if lastContrib2.Valid {
					lastContribOther = &lastContrib2.String
				}
			} else {
				otherUserID = s.UserID1
				if lastContrib2.Valid {
					lastContribSelf = &lastContrib2.String
				}
				if lastContrib1.Valid {
					lastContribOther = &lastContrib1.String
				}
			}

			// Check if streak is active
			isActive := checkStreakActive(lastContribSelf, lastContribOther, todayJournalDate)
			needsSelfPost := needsPost(lastContribSelf, todayJournalDate)
			needsOtherPost := needsPost(lastContribOther, todayJournalDate)

			streaks = append(streaks, StreakWithUser{
				StreakID:              s.ID,
				OtherUserID:           otherUserID,
				OtherUsername:         otherUsername,
				OtherDisplayName:      otherDisplayName,
				StreakCount:           s.StreakCount,
				LastContributionSelf:  lastContribSelf,
				LastContributionOther: lastContribOther,
				StartedAt:             s.StartedAt,
				UpdatedAt:             s.UpdatedAt,
				IsActive:              isActive,
				NeedsSelfPost:         needsSelfPost,
				NeedsOtherPost:        needsOtherPost,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(streaks)
	}
}

// GetStreakBetweenUsers returns the streak between two specific users
// GET /streaks?user1=1&user2=2
func GetStreakBetweenUsers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user1Str := r.URL.Query().Get("user1")
		user2Str := r.URL.Query().Get("user2")

		if user1Str == "" || user2Str == "" {
			http.Error(w, "Both user1 and user2 parameters required", http.StatusBadRequest)
			return
		}

		user1, err := strconv.Atoi(user1Str)
		if err != nil {
			http.Error(w, "Invalid user1 ID", http.StatusBadRequest)
			return
		}

		user2, err := strconv.Atoi(user2Str)
		if err != nil {
			http.Error(w, "Invalid user2 ID", http.StatusBadRequest)
			return
		}

		if user1 == user2 {
			http.Error(w, "Cannot get streak with yourself", http.StatusBadRequest)
			return
		}

		// Ensure user1 < user2 for query
		if user1 > user2 {
			user1, user2 = user2, user1
		}

		var s Streak
		var lastContrib1, lastContrib2 sql.NullString

		err = db.QueryRow(`
			SELECT 
				id,
				user_id_1,
				user_id_2,
				streak_count,
				last_contribution_date_user1,
				last_contribution_date_user2,
				started_at,
				updated_at
			FROM streaks
			WHERE user_id_1 = $1 AND user_id_2 = $2
		`, user1, user2).Scan(
			&s.ID,
			&s.UserID1,
			&s.UserID2,
			&s.StreakCount,
			&lastContrib1,
			&lastContrib2,
			&s.StartedAt,
			&s.UpdatedAt,
		)

		if err == sql.ErrNoRows {
			// No streak exists - return 0 streak
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"streak_count": 0,
				"exists":       false,
			})
			return
		} else if err != nil {
			http.Error(w, "Failed to fetch streak", http.StatusInternalServerError)
			log.Printf("GetStreakBetweenUsers error: %v", err)
			return
		}

		if lastContrib1.Valid {
			s.LastContributionUser1 = &lastContrib1.String
		}
		if lastContrib2.Valid {
			s.LastContributionUser2 = &lastContrib2.String
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"streak":       s,
			"exists":       true,
			"streak_count": s.StreakCount,
		})
	}
}

// UpdateStreaksAfterPost is called after a user creates a post
// This updates all relevant streaks for mutual followers
func UpdateStreaksAfterPost(db *sql.DB, userID int, journalDate time.Time) {
	// Get all mutual followers (users who follow each other)
	rows, err := db.Query(`
		SELECT DISTINCT
			CASE 
				WHEN f1.follower_id = $1 THEN f1.following_id
				ELSE f1.follower_id
			END as other_user_id
		FROM followers f1
		JOIN followers f2 ON (
			(f1.follower_id = $1 AND f1.following_id = f2.follower_id AND f2.following_id = $1)
			OR
			(f1.following_id = $1 AND f1.follower_id = f2.following_id AND f2.follower_id = $1)
		)
		WHERE f1.status = 'accepted' 
		  AND f2.status = 'accepted'
		  AND (f1.follower_id = $1 OR f1.following_id = $1)
	`, userID)

	if err != nil {
		log.Printf("UpdateStreaksAfterPost: Failed to get mutual followers: %v", err)
		return
	}
	defer rows.Close()

	var mutualFollowers []int
	for rows.Next() {
		var otherUserID int
		if err := rows.Scan(&otherUserID); err != nil {
			log.Printf("UpdateStreaksAfterPost: Scan error: %v", err)
			continue
		}
		mutualFollowers = append(mutualFollowers, otherUserID)
	}

	// Update streak with each mutual follower
	for _, otherUserID := range mutualFollowers {
		updateSingleStreak(db, userID, otherUserID, journalDate)
	}
}

// updateSingleStreak updates or creates a streak between two users
func updateSingleStreak(db *sql.DB, userID, otherUserID int, journalDate time.Time) {
	// Ensure user1 < user2
	user1, user2 := userID, otherUserID
	if user1 > user2 {
		user1, user2 = user2, user1
	}

	isUser1 := (userID == user1)

	// Get current streak or create new one
	var streakID int
	var currentCount int
	var lastContrib1, lastContrib2 sql.NullString

	err := db.QueryRow(`
		SELECT id, streak_count, last_contribution_date_user1, last_contribution_date_user2
		FROM streaks
		WHERE user_id_1 = $1 AND user_id_2 = $2
	`, user1, user2).Scan(&streakID, &currentCount, &lastContrib1, &lastContrib2)

	if err == sql.ErrNoRows {
		// Create new streak
		if isUser1 {
			_, err = db.Exec(`
				INSERT INTO streaks (user_id_1, user_id_2, streak_count, last_contribution_date_user1)
				VALUES ($1, $2, 0, $3)
			`, user1, user2, journalDate)
		} else {
			_, err = db.Exec(`
				INSERT INTO streaks (user_id_1, user_id_2, streak_count, last_contribution_date_user2)
				VALUES ($1, $2, 0, $3)
			`, user1, user2, journalDate)
		}
		if err != nil {
			log.Printf("Failed to create streak: %v", err)
		}
		return
	} else if err != nil {
		log.Printf("Failed to query streak: %v", err)
		return
	}

	// Parse last contribution dates
	yesterday := journalDate.AddDate(0, 0, -1)

	var lastSelf, lastOther *time.Time
	if isUser1 {
		if lastContrib1.Valid {
			t, _ := time.Parse("2006-01-02", lastContrib1.String)
			lastSelf = &t
		}
		if lastContrib2.Valid {
			t, _ := time.Parse("2006-01-02", lastContrib2.String)
			lastOther = &t
		}
	} else {
		if lastContrib2.Valid {
			t, _ := time.Parse("2006-01-02", lastContrib2.String)
			lastSelf = &t
		}
		if lastContrib1.Valid {
			t, _ := time.Parse("2006-01-02", lastContrib1.String)
			lastOther = &t
		}
	}

	// Check if we already posted today
	if lastSelf != nil && lastSelf.Equal(journalDate) {
		return // Already posted today, no update needed
	}

	// Calculate new streak count
	newCount := currentCount

	if lastOther != nil {
		// Other user has posted
		if lastOther.Equal(journalDate) {
			// Both posted today - increment if yesterday was maintained
			if lastSelf != nil && lastSelf.Equal(yesterday) {
				newCount++
			} else if currentCount == 0 {
				// Start new streak
				newCount = 1
			} else {
				// Streak was broken, restart
				newCount = 1
			}
		} else if lastOther.Equal(yesterday) {
			// Other user posted yesterday, we're posting today
			if lastSelf != nil && lastSelf.Equal(yesterday) {
				// We both posted yesterday, continue streak
				newCount++
			}
		} else {
			// Other user's last post is too old, reset
			newCount = 0
		}
	} else {
		// Other user hasn't posted yet, keep current count
		newCount = currentCount
	}

	// Update streak
	if isUser1 {
		_, err = db.Exec(`
			UPDATE streaks
			SET streak_count = $1, last_contribution_date_user1 = $2
			WHERE id = $3
		`, newCount, journalDate, streakID)
	} else {
		_, err = db.Exec(`
			UPDATE streaks
			SET streak_count = $1, last_contribution_date_user2 = $2
			WHERE id = $3
		`, newCount, journalDate, streakID)
	}

	if err != nil {
		log.Printf("Failed to update streak: %v", err)
	}
}

// Helper functions
func checkStreakActive(lastSelf, lastOther *string, today time.Time) bool {
	if lastSelf == nil || lastOther == nil {
		return false
	}

	selfDate, err1 := time.Parse("2006-01-02", *lastSelf)
	otherDate, err2 := time.Parse("2006-01-02", *lastOther)

	if err1 != nil || err2 != nil {
		return false
	}

	yesterday := today.AddDate(0, 0, -1)

	// Streak is active if both users posted today or yesterday
	selfRecent := selfDate.Equal(today) || selfDate.Equal(yesterday)
	otherRecent := otherDate.Equal(today) || otherDate.Equal(yesterday)

	return selfRecent && otherRecent
}

func needsPost(lastContribution *string, today time.Time) bool {
	if lastContribution == nil {
		return true
	}

	lastDate, err := time.Parse("2006-01-02", *lastContribution)
	if err != nil {
		return true
	}

	return !lastDate.Equal(today)
}
