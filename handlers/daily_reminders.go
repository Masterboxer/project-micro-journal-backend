package handlers

import (
	"database/sql"
	"log"
	"strconv"
	"time"

	"masterboxer.com/project-micro-journal/services"
)

func SendDailyReminderNotifications(db *sql.DB) {
	nowUTC := time.Now().UTC()

	rows, err := db.Query(`
		SELECT id, timezone
		FROM users
		WHERE timezone IS NOT NULL
	`)

	if err != nil {
		log.Printf("DailyReminder: failed to fetch users: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var userID int
		var timezone string

		if err := rows.Scan(&userID, &timezone); err != nil {
			log.Printf("DailyReminder: scan error: %v", err)
			continue
		}

		loc, err := time.LoadLocation(timezone)
		if err != nil {
			log.Printf("DailyReminder: invalid timezone for user %d: %v", userID, err)
			continue
		}

		localNow := nowUTC.In(loc)

		if localNow.Hour() != 21 {
			continue
		}

		journalDate, err := ComputeJournalDate(nowUTC, timezone)
		if err != nil {
			log.Printf("DailyReminder: journal date error for user %d: %v", userID, err)
			continue
		}

		var exists bool
		err = db.QueryRow(`
			SELECT EXISTS (
				SELECT 1
				FROM posts
				WHERE user_id = $1
				  AND journal_date = $2
			)
		`, userID, journalDate).Scan(&exists)

		if err != nil {
			log.Printf("DailyReminder: post check failed for user %d: %v", userID, err)
			continue
		}

		if exists {
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
			log.Printf("DailyReminder: token fetch failed for user %d: %v", userID, err)
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

		title := "Time to reflect 📝"
		body := "You haven't journaled today. Take a minute for yourself."

		data := map[string]string{
			"type":    "daily_reminder",
			"user_id": strconv.Itoa(userID),
		}

		success, failure, err :=
			services.SendMultipleNotifications(tokens, title, body, data)

		if err != nil {
			log.Printf("DailyReminder: FCM send error for user %d: %v", userID, err)
			continue
		}

		log.Printf(
			"DailyReminder: user %d → %d sent, %d failed",
			userID, success, failure,
		)
	}
}
