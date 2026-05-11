package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"masterboxer.com/project-micro-journal/database"
	"masterboxer.com/project-micro-journal/routes"
	"masterboxer.com/project-micro-journal/services"
)

func main() {
	db, err := database.ConnectDB()
	if err != nil {
		log.Fatal("Failed to connect to the database:", err)
	}
	defer db.Close()

	if err := services.InitFirebase("./project-micro-journal-firebase-adminsdk-fbsvc-e626a40f9b.json"); err != nil {
		log.Printf("Warning: Firebase initialization failed: %v", err)
	}

	cfg := services.Config{
		Host:     os.Getenv("SMTP_HOST"),
		Port:     587,
		Username: os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASS"),
		From:     os.Getenv("SMTP_FROM"),
	}

	mailSvc, err := services.NewMailService(cfg)
	if err != nil {
		log.Fatal(err)
	}

	router := mux.NewRouter()

	routes.CreateUserRoutes(db, router)
	routes.CreateAuthenticationRoutes(db, mailSvc, router)
	routes.CreatePostRoutes(db, router)
	routes.CreateMailVerificationRoutes(db, mailSvc, router)
	routes.CreateTemplateRoutes(db, router)
	routes.CreateNotificationRoutes(db, router)

	handler := corsMiddleware(jsonContentTypeMiddleware(router))

	log.Println("Starting server on :8200...")
	log.Fatal(http.ListenAndServe(":8200", handler))
}

func jsonContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5000")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
