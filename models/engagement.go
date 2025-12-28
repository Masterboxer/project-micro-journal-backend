package models

import "time"

type Comment struct {
	ID        int       `json:"id"`
	PostID    int       `json:"post_id"`
	UserID    int       `json:"user_id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type CommentWithUser struct {
	ID          int       `json:"id"`
	PostID      int       `json:"post_id"`
	UserID      int       `json:"user_id"`
	Text        string    `json:"text"`
	CreatedAt   time.Time `json:"created_at"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
}

type Like struct {
	ID        int       `json:"id"`
	PostID    int       `json:"post_id"`
	UserID    int       `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

type PostWithEngagement struct {
	PostWithUser
	LikeCount     int  `json:"like_count"`
	CommentCount  int  `json:"comment_count"`
	IsLikedByUser bool `json:"is_liked_by_user"`
}
