package handlers

import (
	"database/sql"
	"log"
	"strconv"
	"time"

	"masterboxer.com/project-micro-journal/services"
)

func SendScoreDecayNotifications(db *sql.DB) {
	nowUTC := time.Now().UTC()
	log.Printf("[ScoreDecayReminder] Job started | nowUTC=%s", nowUTC.Format(time.RFC3339))

	rows, err := db.Query(`
		SELECT rs.user_id, rs.last_post_date, rs.score, u.timezone
		FROM reflecto_scores rs
		JOIN users u ON u.id = rs.user_id
		WHERE rs.score > 0
		  AND u.timezone IS NOT NULL
	`)
	if err != nil {
		log.Printf("[ScoreDecayReminder] DB query failed: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var userID int
		var lastPostDate sql.NullString
		var score int
		var timezone string

		if err := rows.Scan(&userID, &lastPostDate, &score, &timezone); err != nil {
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

		// Skip if they've already posted today
		if lastPostDate.Valid {
			t, err := time.Parse("2006-01-02", lastPostDate.String[:10])
			if err == nil && !t.Before(journalToday) {
				continue
			}
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
			"📉 Your Reflecto Score is at risk!",
			"Post today to protect your score of "+strconv.Itoa(score)+" — missing a day costs you a point.",
			map[string]string{
				"type":    "score_decay_warning",
				"user_id": strconv.Itoa(userID),
			},
		)
		if err != nil {
			log.Printf("[ScoreDecayReminder] FCM error user=%d: %v", userID, err)
			continue
		}

		log.Printf(
			"[ScoreDecayReminder] Sent decay warning | user=%d score=%d success=%d failure=%d",
			userID, score, success, failure,
		)
	}

	log.Println("[ScoreDecayReminder] Job finished")
}
