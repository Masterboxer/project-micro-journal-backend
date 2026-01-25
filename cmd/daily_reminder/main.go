package main

import (
	"log"
	"os"

	"masterboxer.com/project-micro-journal/database"
	"masterboxer.com/project-micro-journal/handlers"
	"masterboxer.com/project-micro-journal/services"
)

func main() {
	firebasePath := os.Getenv("FIREBASE_CREDENTIALS_PATH")
	if firebasePath == "" {
		log.Fatal("FIREBASE_CREDENTIALS_PATH not set")
	}

	if err := services.InitFirebase(firebasePath); err != nil {
		log.Printf("DailyReminder: Firebase init failed: %v", err)
	}

	db, err := database.ConnectDB()
	if err != nil {
		log.Fatal("DailyReminder: DB connection failed:", err)
	}
	defer db.Close()

	log.Println("⏰ Running daily reminder job")
	handlers.SendDailyReminderNotifications(db)
	log.Println("✅ Daily reminder job finished")
}
