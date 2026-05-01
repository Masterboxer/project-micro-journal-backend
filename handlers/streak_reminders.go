package handlers

import (
	"database/sql"
	"log"
	"strconv"
	"time"

	"masterboxer.com/project-micro-journal/services"
)

func SendStreakExpiryNotifications(db *sql.DB) {
	nowUTC := time.Now().UTC()
	log.Printf("[StreakReminder] Job started | nowUTC=%s", nowUTC.Format(time.RFC3339))

	rows, err := db.Query(`
		SELECT s.user_id, s.last_post_date, u.timezone
		FROM streaks s
		JOIN users u ON u.id = s.user_id
		WHERE s.streak_count > 0
		  AND s.last_post_date IS NOT NULL
		  AND u.timezone IS NOT NULL
	`)
	if err != nil {
		log.Printf("[StreakReminder] DB query failed: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var userID int
		var lastPostDate time.Time
		var timezone string

		if err := rows.Scan(&userID, &lastPostDate, &timezone); err != nil {
			continue
		}

		loc, err := time.LoadLocation(timezone)
		if err != nil {
			continue
		}

		localNow := nowUTC.In(loc)

		// Only between 11:00 AM and 11:15 AM local time
		if localNow.Hour() != 11 || localNow.Minute() > 15 {
			continue
		}

		journalToday, err := ComputeJournalDate(nowUTC, timezone)
		if err != nil {
			continue
		}

		lastPostDateOnly := time.Date(
			lastPostDate.Year(),
			lastPostDate.Month(),
			lastPostDate.Day(),
			0, 0, 0, 0,
			time.UTC,
		)

		if !lastPostDateOnly.Before(journalToday) {
			continue
		}

		yesterday := journalToday.AddDate(0, 0, -1)
		if !lastPostDateOnly.Equal(yesterday) {
			continue
		}

		tokenRows, err := db.Query(`
			SELECT token
			FROM fcm_tokens
			WHERE user_id = $1
			  AND token IS NOT NULL
			  AND token != ''
		`, userID)
		if err != nil {
			continue
		}

		var tokens []string
		for tokenRows.Next() {
			var token string
			if err := tokenRows.Scan(&token); err == nil {
				tokens = append(tokens, token)
			}
		}
		tokenRows.Close()

		if len(tokens) == 0 {
			continue
		}

		success, failure, err := services.SendMultipleNotifications(
			db,
			tokens,
			"🔥 Don't let your streak expire today!",
			"You have less than an hour to post and keep your streak alive, so let's do that now?",
			map[string]string{
				"type":    "streak_expiry",
				"user_id": strconv.Itoa(userID),
			},
		)

		if err != nil {
			log.Printf("[StreakReminder] FCM error user=%d: %v", userID, err)
			continue
		}

		log.Printf(
			"[StreakReminder] Sent streak expiry warning | user=%d success=%d failure=%d",
			userID, success, failure,
		)
	}

	log.Println("[StreakReminder] Job finished")
}
