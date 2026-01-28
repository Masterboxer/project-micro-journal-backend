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
	log.Printf("[DailyReminder] Job started at %v UTC", nowUTC)

	rows, err := db.Query(`
        SELECT id, timezone
        FROM users
        WHERE timezone IS NOT NULL
    `)
	if err != nil {
		log.Printf("[DailyReminder] Failed to fetch users: %v", err)
		return
	}
	defer rows.Close()

	var processedUsers int
	var notificationsSent int

	for rows.Next() {
		var userID int
		var timezone string
		if err := rows.Scan(&userID, &timezone); err != nil {
			log.Printf("[DailyReminder] Scan error: %v", err)
			continue
		}

		log.Printf("[DailyReminder] Processing user %d with timezone %s", userID, timezone)

		loc, err := time.LoadLocation(timezone)
		if err != nil {
			log.Printf("[DailyReminder] Invalid timezone for user %d: %v", userID, err)
			continue
		}

		localNow := nowUTC.In(loc)
		log.Printf("[DailyReminder] User %d local time: %v (hour=%d, minute=%d)",
			userID, localNow, localNow.Hour(), localNow.Minute())

		// 9:00–9:05 PM local time window
		if localNow.Hour() != 21 || localNow.Minute() > 5 {
			log.Printf("[DailyReminder] User %d outside time window (need 21:00-21:05, got %02d:%02d)",
				userID, localNow.Hour(), localNow.Minute())
			continue
		}

		journalDate, err := ComputeJournalDate(nowUTC, timezone)
		if err != nil {
			log.Printf("[DailyReminder] Journal date error for user %d: %v", userID, err)
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
			log.Printf("[DailyReminder] Database error checking posts for user %d: %v", userID, err)
			continue
		}

		if exists {
			log.Printf("[DailyReminder] User %d already posted for journal date %v", userID, journalDate)
			continue
		}

		log.Printf("[DailyReminder] User %d has not posted, fetching tokens...", userID)

		rowsTokens, err := db.Query(`
            SELECT token FROM fcm_tokens
            WHERE user_id = $1 AND token IS NOT NULL AND token != ''
        `, userID)
		if err != nil {
			log.Printf("[DailyReminder] Error fetching tokens for user %d: %v", userID, err)
			continue
		}

		var tokens []string
		for rowsTokens.Next() {
			var token string
			if err := rowsTokens.Scan(&token); err == nil {
				tokens = append(tokens, token)
			}
		}
		rowsTokens.Close()

		if len(tokens) == 0 {
			log.Printf("[DailyReminder] No FCM tokens found for user %d", userID)
			continue
		}

		log.Printf("[DailyReminder] Sending notification to user %d with %d token(s)", userID, len(tokens))

		success, failure, err := services.SendMultipleNotifications(
			db,
			tokens,
			"Time to reflect 📝",
			"You haven't added to your micro journal today. Take a minute for yourself and your loved ones",
			map[string]string{
				"type":    "daily_reminder",
				"user_id": strconv.Itoa(userID),
			},
		)
		if err != nil {
			log.Printf("[DailyReminder] FCM error for user %d: %v", userID, err)
			continue
		}

		processedUsers++
		notificationsSent += success

		log.Printf(
			"[DailyReminder] User %d → %d sent, %d failed",
			userID, success, failure,
		)
	}

	log.Printf("[DailyReminder] Job finished | Processed %d users, sent %d notifications",
		processedUsers, notificationsSent)
}
