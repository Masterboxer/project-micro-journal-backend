package routes

import (
	"database/sql"

	"github.com/gorilla/mux"
	"masterboxer.com/project-micro-journal/handlers"
	"masterboxer.com/project-micro-journal/services"
)

func CreateAuthenticationRoutes(db *sql.DB, mailSvc *services.MailService, router *mux.Router) *mux.Router {

	router.HandleFunc("/login", handlers.LoginHandler(db)).Methods("POST")
	router.HandleFunc("/logout", handlers.LogoutHandler(db)).Methods("POST")
	router.HandleFunc("/verify-token", handlers.VerifyTokenHandler(db)).Methods("POST")
	router.HandleFunc("/refresh-token", handlers.RefreshTokenHandler(db)).Methods("POST")
	router.HandleFunc("/auth/google", handlers.GoogleSignInHandler(db)).Methods("POST")
	router.HandleFunc("/auth/google/complete", handlers.CompleteGoogleSignUp(db)).Methods("POST")
	router.HandleFunc("/forgot-password", handlers.ForgotPasswordHandler(db, mailSvc)).Methods("POST")
	router.HandleFunc("/validate-reset-token", handlers.ValidateResetTokenHandler(db)).Methods("POST")
	router.HandleFunc("/reset-password", handlers.ResetPasswordHandler(db)).Methods("POST")

	return router
}
