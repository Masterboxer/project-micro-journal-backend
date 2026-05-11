package routes

import (
	"database/sql"

	"github.com/gorilla/mux"
	"masterboxer.com/project-micro-journal/handlers"
	"masterboxer.com/project-micro-journal/services"
)

func CreateMailVerificationRoutes(db *sql.DB, mailSvc *services.MailService, router *mux.Router) *mux.Router {
	router.HandleFunc("/verify-email", handlers.VerifyEmailHandler(db)).Methods("GET")
	router.HandleFunc("/send-verification-mail", handlers.SendVerificationEmailHandler(db, mailSvc)).Methods("POST")
	router.HandleFunc("/resend-verification-mail", handlers.ResendVerificationEmailHandler(db, mailSvc)).Methods("POST")
	return router
}
