package services

import (
	"context"
	"database/sql"
	"log"
	"sync"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

var (
	messagingClient *messaging.Client
	once            sync.Once
	initError       error
)

func InitFirebase(credentialsPath string) error {
	once.Do(func() {
		ctx := context.Background()

		log.Printf("[FCM] Initializing Firebase with credentials: %s", credentialsPath)

		opt := option.WithCredentialsFile(credentialsPath)
		app, err := firebase.NewApp(ctx, nil, opt)
		if err != nil {
			initError = err
			log.Printf("[FCM][ERROR] Failed to init Firebase app: %v", err)
			return
		}

		messagingClient, err = app.Messaging(ctx)
		if err != nil {
			initError = err
			log.Printf("[FCM][ERROR] Failed to get messaging client: %v", err)
			return
		}

		log.Println("[FCM] Firebase Messaging client initialized successfully")
	})

	return initError
}

func GetMessagingClient() (*messaging.Client, error) {
	if messagingClient == nil {
		log.Printf("[FCM][ERROR] Messaging client is nil (initError=%v)", initError)
		return nil, initError
	}
	return messagingClient, nil
}

func SendNotification(deviceToken, title, body string, data map[string]string) error {
	client, err := GetMessagingClient()
	if err != nil {
		return err
	}

	message := &messaging.Message{
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data:  data,
		Token: deviceToken,
	}

	response, err := client.Send(context.Background(), message)
	if err != nil {
		log.Printf("Error sending notification: %v", err)
		return err
	}

	log.Printf("Successfully sent message: %s", response)
	return nil
}

func SendMultipleNotifications(
	db *sql.DB,
	tokens []string,
	title, body string,
	data map[string]string,
) (int, int, error) {

	client, err := GetMessagingClient()
	if err != nil {
		return 0, 0, err
	}

	log.Printf(
		"[FCM] Sending multicast | tokens=%d title=%q",
		len(tokens),
		title,
	)

	// Log first token (helps debugging project mismatch)
	if len(tokens) > 0 {
		log.Printf("[FCM] Sample token: %s...", tokens[0][:min(10, len(tokens[0]))])
	}

	message := &messaging.MulticastMessage{
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data:   data,
		Tokens: tokens,
	}

	response, err := client.SendEachForMulticast(context.Background(), message)
	if err != nil {
		log.Printf("[FCM][ERROR] Multicast send failed entirely: %v", err)
		return 0, 0, err
	}

	log.Printf(
		"[FCM] Multicast result | success=%d failure=%d",
		response.SuccessCount,
		response.FailureCount,
	)

	// 🔥 PER-TOKEN ERROR LOGGING (CRITICAL)
	for i, resp := range response.Responses {
		if resp.Success {
			continue
		}

		token := tokens[i]
		log.Printf(
			"[FCM][TOKEN ERROR] token=%s error=%v",
			token,
			resp.Error,
		)

		// Detect dead tokens explicitly
		if messaging.IsUnregistered(resp.Error) {
			log.Printf("[FCM] Deleting dead token: %s", token)

			_, err := db.Exec(`
					DELETE FROM fcm_tokens
					WHERE token = $1
				`, token)

			if err != nil {
				log.Printf("[FCM][ERROR] Failed to delete token %s: %v", token, err)
			}
		}

	}

	return response.SuccessCount, response.FailureCount, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func SendNotificationToUser(db interface{}, userID int, title, body string, data map[string]string) error {
	return nil
}
