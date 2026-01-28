package routes

import (
	"database/sql"

	"github.com/gorilla/mux"
	"masterboxer.com/project-micro-journal/handlers"
)

func CreatePostRoutes(db *sql.DB, router *mux.Router) *mux.Router {
	router.HandleFunc("/posts/today", handlers.GetTodayPostForUser(db)).Methods("GET")
	router.HandleFunc("/posts", handlers.CreatePost(db)).Methods("POST")
	router.HandleFunc("/posts/user/{userId}", handlers.GetPostsByUser(db)).Methods("GET")
	router.HandleFunc("/posts/{id}", handlers.DeletePost(db)).Methods("DELETE")
	router.HandleFunc("/posts/{userId}/feed", handlers.GetUserFeed(db)).Methods("GET")
	router.HandleFunc("/posts/{postId}/react", handlers.AddReaction(db)).Methods("POST")
	router.HandleFunc("/posts/{postId}/reacts", handlers.GetPostReactions(db)).Methods("GET")
	router.HandleFunc("/posts/{postId}/comments", handlers.CreateComment(db)).Methods("POST")
	router.HandleFunc("/posts/{postId}/comments", handlers.GetPostComments(db)).Methods("GET")
	router.HandleFunc("/comments/{commentId}", handlers.DeleteComment(db)).Methods("DELETE")

	return router
}
