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

	router.HandleFunc("/users/{user_id}/follow", handlers.FollowUser(db)).Methods("POST")
	router.HandleFunc("/users/{user_id}/following/{following_id}", handlers.UnfollowUser(db)).Methods("DELETE")
	router.HandleFunc("/users/{user_id}/followers/{follower_id}", handlers.RemoveFollower(db)).Methods("DELETE")
	router.HandleFunc("/users/{user_id}/disconnect/{target_user_id}", handlers.UnfollowAndRemove(db)).Methods("DELETE")
	router.HandleFunc("/users/{user_id}/followers", handlers.GetUserFollowers(db)).Methods("GET")
	router.HandleFunc("/users/{user_id}/following", handlers.GetUserFollowing(db)).Methods("GET")
	router.HandleFunc("/users/{user_id}/follow-stats", handlers.GetFollowStats(db)).Methods("GET")
	router.HandleFunc("/users/{user_id}/follow-status/{target_user_id}", handlers.CheckFollowStatus(db)).Methods("GET")

	router.HandleFunc("/users/{user_id}/follow-requests/pending", handlers.GetPendingFollowRequests(db)).Methods("GET")
	router.HandleFunc("/users/{user_id}/follow-requests/sent", handlers.GetSentFollowRequests(db)).Methods("GET")
	router.HandleFunc("/users/{user_id}/follow-requests/{follower_id}/accept", handlers.AcceptFollowRequest(db)).Methods("POST")
	router.HandleFunc("/users/{user_id}/follow-requests/{follower_id}/reject", handlers.RejectFollowRequest(db)).Methods("POST")
	router.HandleFunc("/users/{user_id}/follow-requests/{following_id}/cancel", handlers.CancelFollowRequest(db)).Methods("DELETE")
	router.HandleFunc("/users/{id}/privacy", handlers.UpdateUserPrivacy(db)).Methods("PUT")

	router.HandleFunc("/users/{userId}/streaks", handlers.GetUserStreaks(db)).Methods("GET")
	router.HandleFunc("/streaks", handlers.GetStreakBetweenUsers(db)).Methods("GET")

	return router
}
