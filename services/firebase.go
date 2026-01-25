package services

import (
	"context"
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
		opt := option.WithCredentialsFile(credentialsPath)
		app, err := firebase.NewApp(ctx, nil, opt)
		if err != nil {
			initError = err
			log.Printf("Error initializing Firebase app: %v", err)
			return
		}

		messagingClient, err = app.Messaging(ctx)
		if err != nil {
			initError = err
			log.Printf("Error getting Messaging client: %v", err)
			return
		}

		log.Println("Firebase Messaging client initialized successfully")
	})
	return initError
}

func GetMessagingClient() (*messaging.Client, error) {
	if messagingClient == nil {
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

func SendMultipleNotifications(tokens []string, title, body string, data map[string]string) (int, int, error) {
	client, err := GetMessagingClient()
	if err != nil {
		return 0, 0, err
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
		log.Printf("Error sending multicast: %v", err)
		return 0, 0, err
	}

	log.Printf("Success: %d, Failure: %d", response.SuccessCount, response.FailureCount)
	return response.SuccessCount, response.FailureCount, nil
}

func SendNotificationToUser(db interface{}, userID int, title, body string, data map[string]string) error {
	return nil
}
