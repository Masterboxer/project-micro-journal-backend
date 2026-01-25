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

	db, err := database.ConnectDB()
	if err != nil {
		log.Fatal("StreakReminder: DB connection failed:", err)
	}
	defer db.Close()

	if err := services.InitFirebase(firebasePath); err != nil {
		log.Printf("StreakReminder: Firebase init failed: %v", err)
	}

	log.Println("🔥 Running streak expiry reminder job")
	handlers.SendStreakExpiryNotifications(db)
	log.Println("✅ Streak expiry reminder job finished")
}
