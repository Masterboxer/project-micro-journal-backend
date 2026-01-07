package models

import "time"

type Post struct {
	ID          int       `json:"id"`
	UserID      int       `json:"user_id"`
	TemplateID  int       `json:"template_id"`
	Text        string    `json:"text"`
	PhotoPath   string    `json:"photo_path,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	JournalDate time.Time `json:"journal_date"`
}

type PostWithUser struct {
	ID          int    `json:"id"`
	UserID      int    `json:"user_id"`
	TemplateID  int    `json:"template_id"`
	Text        string `json:"text"`
	PhotoPath   string `json:"photo_path"`
	CreatedAt   string `json:"created_at"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}
