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
	log.Printf("[DailyReminder] Job started at UTC: %v", nowUTC)

	rows, err := db.Query(`
		SELECT id, timezone
		FROM users
		WHERE timezone IS NOT NULL
	`)
	if err != nil {
		log.Printf("[DailyReminder] ❌ Failed to fetch users: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var userID int
		var timezone string

		if err := rows.Scan(&userID, &timezone); err != nil {
			log.Printf("[DailyReminder] ❌ Scan error: %v", err)
			continue
		}

		log.Printf("[DailyReminder] 👤 Checking user %d (tz=%s)", userID, timezone)

		loc, err := time.LoadLocation(timezone)
		if err != nil {
			log.Printf("[DailyReminder] ❌ Invalid timezone for user %d: %v", userID, err)
			continue
		}

		localNow := nowUTC.In(loc)
		log.Printf(
			"[DailyReminder] 🕒 User %d local time: %02d:%02d",
			userID,
			localNow.Hour(),
			localNow.Minute(),
		)

		if localNow.Hour() != 21 || localNow.Minute() > 5 {
			continue
		}

		journalDate, err := ComputeJournalDate(nowUTC, timezone)
		if err != nil {
			log.Printf("[DailyReminder] ❌ Journal date error for user %d: %v", userID, err)
			continue
		}

		var exists bool
		err = db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM posts
				WHERE user_id = $1 AND journal_date = $2
			)
		`, userID, journalDate).Scan(&exists)

		if err != nil {
			log.Printf("[DailyReminder] ❌ Post check failed for user %d: %v", userID, err)
			continue
		}

		if exists {
			log.Printf("[DailyReminder] ⏭️ User %d already posted today", userID)
			continue
		}

		tokenRows, err := db.Query(`
			SELECT token FROM fcm_tokens
			WHERE user_id = $1
			  AND token IS NOT NULL
			  AND token != ''
		`, userID)
		if err != nil {
			log.Printf("[DailyReminder] ❌ Token fetch failed for user %d: %v", userID, err)
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
			log.Printf("[DailyReminder] ⏭️ User %d has NO FCM tokens", userID)
			continue
		}

		log.Printf("[DailyReminder] 📲 Sending notification to user %d (%d tokens)", userID, len(tokens))

		title := "Time to reflect 📝"
		body := "You haven't journaled today. Take a minute for yourself."

		data := map[string]string{
			"type":    "daily_reminder",
			"user_id": strconv.Itoa(userID),
		}

		success, failure, err :=
			services.SendMultipleNotifications(tokens, title, body, data)

		if err != nil {
			log.Printf("[DailyReminder] ❌ FCM error for user %d: %v", userID, err)
			continue
		}

		log.Printf(
			"[DailyReminder] ✅ User %d → %d sent, %d failed",
			userID, success, failure,
		)
	}

	log.Println("[DailyReminder] Job finished")
}
