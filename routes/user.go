package routes

import (
	"database/sql"

	"github.com/gorilla/mux"
	"masterboxer.com/project-micro-journal/handlers"
)

func CreateUserRoutes(db *sql.DB, router *mux.Router) *mux.Router {

	router.HandleFunc("/users/search", handlers.SearchUsers(db)).Methods("GET")
	router.HandleFunc("/users", handlers.GetUsers(db)).Methods("GET")
	router.HandleFunc("/users/{id}", handlers.GetUserById(db)).Methods("GET")
	router.HandleFunc("/users/{id}", handlers.UpdateUser(db)).Methods("PUT")
	router.HandleFunc("/users/{id}", handlers.DeleteUser(db)).Methods("DELETE")
	router.HandleFunc("/users", handlers.CreateUser(db)).Methods("POST")

	router.HandleFunc("/users/{user_id}/buddy-requests", handlers.SendBuddyRequest(db)).Methods("POST")
	router.HandleFunc("/users/{user_id}/buddy-requests/received", handlers.GetReceivedBuddyRequests(db)).Methods("GET")
	router.HandleFunc("/users/{user_id}/buddy-requests/sent", handlers.GetSentBuddyRequests(db)).Methods("GET")
	router.HandleFunc("/buddy-requests/{request_id}/accept", handlers.AcceptBuddyRequest(db)).Methods("POST")
	router.HandleFunc("/buddy-requests/{request_id}/reject", handlers.RejectBuddyRequest(db)).Methods("POST")
	router.HandleFunc("/buddy-requests/{request_id}/cancel", handlers.CancelBuddyRequest(db)).Methods("DELETE")

	router.HandleFunc("/users/{user_id}/buddies", handlers.GetUserBuddies(db)).Methods("GET")
	router.HandleFunc("/users/{user_id}/buddies/{buddy_id}", handlers.RemoveBuddy(db)).Methods("DELETE")

	return router
}
