package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"masterboxer.com/project-micro-journal/models"
	"masterboxer.com/project-micro-journal/services"
)

func SendBuddyRequest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		requesterID, _ := strconv.Atoi(vars["user_id"])

		var req struct {
			RecipientID int `json:"recipient_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.RecipientID == requesterID {
			http.Error(w, "Cannot send buddy request to yourself", http.StatusBadRequest)
			return
		}

		var recipientExists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", req.RecipientID).Scan(&recipientExists)
		if err != nil || !recipientExists {
			http.Error(w, "Recipient user not found", http.StatusNotFound)
			return
		}

		var alreadyBuddies bool
		err = db.QueryRow(`
            SELECT EXISTS(
                SELECT 1 FROM buddies 
                WHERE (user_id = $1 AND buddy_id = $2) 
                   OR (user_id = $2 AND buddy_id = $1)
            )`, requesterID, req.RecipientID).Scan(&alreadyBuddies)

		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if alreadyBuddies {
			http.Error(w, "Already buddies with this user", http.StatusConflict)
			return
		}

		var existingRequest models.BuddyRequest
		err = db.QueryRow(`
            SELECT id, status FROM buddy_requests 
            WHERE (requester_id = $1 AND recipient_id = $2)
               OR (requester_id = $2 AND recipient_id = $1)`,
			requesterID, req.RecipientID).Scan(&existingRequest.ID, &existingRequest.Status)

		if err == nil {
			// Request exists
			if existingRequest.Status == "pending" {
				http.Error(w, "Buddy request already pending", http.StatusConflict)
				return
			} else if existingRequest.Status == "rejected" {
				_, err = db.Exec(`
                    UPDATE buddy_requests 
                    SET status = 'pending', updated_at = NOW() 
                    WHERE id = $1`, existingRequest.ID)
				if err != nil {
					http.Error(w, "Failed to resend buddy request", http.StatusInternalServerError)
					return
				}
			}
		} else if err != sql.ErrNoRows {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		} else {
			err = db.QueryRow(`
                INSERT INTO buddy_requests (requester_id, recipient_id, status, created_at, updated_at)
                VALUES ($1, $2, 'pending', NOW(), NOW())
                RETURNING id`,
				requesterID, req.RecipientID).Scan(&existingRequest.ID)

			if err != nil {
				http.Error(w, "Failed to send buddy request", http.StatusInternalServerError)
				log.Println("SendBuddyRequest error:", err)
				return
			}
		}

		go notifyBuddyRequest(db, requesterID, req.RecipientID)

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":    "Buddy request sent successfully",
			"request_id": existingRequest.ID,
		})
	}
}

func GetReceivedBuddyRequests(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])

		rows, err := db.Query(`
            SELECT br.id, br.requester_id, br.status, br.created_at, br.updated_at,
                   u.username, u.display_name
            FROM buddy_requests br
            JOIN users u ON br.requester_id = u.id
            WHERE br.recipient_id = $1 AND br.status = 'pending'
            ORDER BY br.created_at DESC`,
			userID)

		if err != nil {
			http.Error(w, "Failed to fetch buddy requests", http.StatusInternalServerError)
			log.Println("GetReceivedBuddyRequests error:", err)
			return
		}
		defer rows.Close()

		var requests []models.BuddyRequestWithUser
		for rows.Next() {
			var req models.BuddyRequestWithUser
			if err := rows.Scan(&req.ID, &req.UserID, &req.Status, &req.CreatedAt,
				&req.UpdatedAt, &req.Username, &req.DisplayName); err != nil {
				http.Error(w, "Error scanning requests", http.StatusInternalServerError)
				return
			}
			requests = append(requests, req)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(requests)
	}
}

func GetSentBuddyRequests(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, _ := strconv.Atoi(vars["user_id"])

		rows, err := db.Query(`
            SELECT br.id, br.recipient_id, br.status, br.created_at, br.updated_at,
                   u.username, u.display_name
            FROM buddy_requests br
            JOIN users u ON br.recipient_id = u.id
            WHERE br.requester_id = $1
            ORDER BY br.created_at DESC`,
			userID)

		if err != nil {
			http.Error(w, "Failed to fetch sent requests", http.StatusInternalServerError)
			log.Println("GetSentBuddyRequests error:", err)
			return
		}
		defer rows.Close()

		var requests []models.BuddyRequestWithUser
		for rows.Next() {
			var req models.BuddyRequestWithUser
			if err := rows.Scan(&req.ID, &req.UserID, &req.Status, &req.CreatedAt,
				&req.UpdatedAt, &req.Username, &req.DisplayName); err != nil {
				http.Error(w, "Error scanning requests", http.StatusInternalServerError)
				return
			}
			requests = append(requests, req)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(requests)
	}
}

func AcceptBuddyRequest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		requestID, _ := strconv.Atoi(vars["request_id"])

		var req struct {
			UserID int `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		var requesterID, recipientID int
		var status string
		err := db.QueryRow(`
            SELECT requester_id, recipient_id, status 
            FROM buddy_requests 
            WHERE id = $1`, requestID).Scan(&requesterID, &recipientID, &status)

		if err == sql.ErrNoRows {
			http.Error(w, "Buddy request not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if recipientID != req.UserID {
			http.Error(w, "Unauthorized to accept this request", http.StatusForbidden)
			return
		}

		if status != "pending" {
			http.Error(w, "Request already processed", http.StatusConflict)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Transaction error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		_, err = tx.Exec(`
            UPDATE buddy_requests 
            SET status = 'accepted', updated_at = NOW() 
            WHERE id = $1`, requestID)
		if err != nil {
			http.Error(w, "Failed to accept request", http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec(`
            INSERT INTO buddies (user_id, buddy_id, created_at)
            VALUES ($1, $2, NOW()), ($2, $1, NOW())
            ON CONFLICT (user_id, buddy_id) DO NOTHING`,
			requesterID, recipientID)
		if err != nil {
			http.Error(w, "Failed to create buddy relationship", http.StatusInternalServerError)
			log.Println("AcceptBuddyRequest buddies insert error:", err)
			return
		}

		if err = tx.Commit(); err != nil {
			http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
			return
		}

		go notifyRequestAccepted(db, requesterID, recipientID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Buddy request accepted",
		})
	}
}

func RejectBuddyRequest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		requestID, _ := strconv.Atoi(vars["request_id"])

		var req struct {
			UserID int `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		var recipientID int
		var status string
		err := db.QueryRow(`
            SELECT recipient_id, status 
            FROM buddy_requests 
            WHERE id = $1`, requestID).Scan(&recipientID, &status)

		if err == sql.ErrNoRows {
			http.Error(w, "Buddy request not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if recipientID != req.UserID {
			http.Error(w, "Unauthorized to reject this request", http.StatusForbidden)
			return
		}

		if status != "pending" {
			http.Error(w, "Request already processed", http.StatusConflict)
			return
		}

		_, err = db.Exec(`
            UPDATE buddy_requests 
            SET status = 'rejected', updated_at = NOW() 
            WHERE id = $1`, requestID)
		if err != nil {
			http.Error(w, "Failed to reject request", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Buddy request rejected",
		})
	}
}

func CancelBuddyRequest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		requestID, _ := strconv.Atoi(vars["request_id"])

		var req struct {
			UserID int `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		var requesterID int
		err := db.QueryRow(`
            SELECT requester_id 
            FROM buddy_requests 
            WHERE id = $1 AND status = 'pending'`, requestID).Scan(&requesterID)

		if err == sql.ErrNoRows {
			http.Error(w, "Pending buddy request not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if requesterID != req.UserID {
			http.Error(w, "Unauthorized to cancel this request", http.StatusForbidden)
			return
		}

		_, err = db.Exec(`DELETE FROM buddy_requests WHERE id = $1`, requestID)
		if err != nil {
			http.Error(w, "Failed to cancel request", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Buddy request cancelled",
		})
	}
}

func notifyBuddyRequest(db *sql.DB, requesterID, recipientID int) {
	var requesterName string
	err := db.QueryRow("SELECT display_name FROM users WHERE id = $1", requesterID).Scan(&requesterName)
	if err != nil {
		log.Printf("Error getting requester name: %v", err)
		requesterName = "Someone"
	}

	rows, err := db.Query(`
        SELECT token FROM fcm_tokens 
        WHERE user_id = $1 AND token IS NOT NULL AND token != ''`,
		recipientID)
	if err != nil {
		log.Printf("Error fetching recipient FCM tokens: %v", err)
		return
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			continue
		}
		tokens = append(tokens, token)
	}

	if len(tokens) > 0 {
		data := map[string]string{
			"type":         "buddy_request",
			"requester_id": strconv.Itoa(requesterID),
		}
		services.SendMultipleNotifications(tokens, "New Buddy Request",
			requesterName+" wants to be your buddy!", data)
	}
}

func notifyRequestAccepted(db *sql.DB, requesterID, accepterID int) {
	var accepterName string
	db.QueryRow("SELECT display_name FROM users WHERE id = $1", accepterID).Scan(&accepterName)

	rows, _ := db.Query(`
        SELECT token FROM fcm_tokens 
        WHERE user_id = $1 AND token IS NOT NULL AND token != ''`, requesterID)
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var token string
		rows.Scan(&token)
		tokens = append(tokens, token)
	}

	if len(tokens) > 0 {
		data := map[string]string{
			"type":    "request_accepted",
			"user_id": strconv.Itoa(accepterID),
		}
		services.SendMultipleNotifications(tokens, "Buddy Request Accepted",
			accepterName+" accepted your buddy request!", data)
	}
}
