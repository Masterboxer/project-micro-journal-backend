package routes

import (
	"database/sql"

	"github.com/gorilla/mux"
	"masterboxer.com/project-micro-journal/handlers"
)

func CreateAuthenticationRoutes(db *sql.DB, router *mux.Router) *mux.Router {

	router.HandleFunc("/login", handlers.LoginHandler(db)).Methods("POST")
	router.HandleFunc("/logout", handlers.LogoutHandler(db)).Methods("POST")
	router.HandleFunc("/verify-token", handlers.VerifyTokenHandler(db)).Methods("POST")
	router.HandleFunc("/refresh-token", handlers.RefreshTokenHandler(db)).Methods("POST")
	router.HandleFunc("/auth/google", handlers.GoogleSignInHandler(db)).Methods("POST")
	router.HandleFunc("/auth/google/complete", handlers.CompleteGoogleSignUp(db)).Methods("POST")

	return router
}
