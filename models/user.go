package models

import (
	"database/sql/driver"
	"fmt"
	"time"
)

type User struct {
	ID          int       `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	DOB         CivilDate `json:"dob"`
	Gender      string    `json:"gender"`
	Email       string    `json:"email"`
	Password    string    `json:"password,omitempty"`
	FCMToken    string    `json:"fcm_token,omitempty"`
	CreatedAt   string    `json:"created_at"`
}

type Buddy struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	BuddyID   int    `json:"buddy_id"`
	CreatedAt string `json:"created_at"`
}

type UserBuddies struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type CivilDate time.Time

func (c CivilDate) MarshalJSON() ([]byte, error) {
	if time.Time(c).IsZero() {
		return []byte(`""`), nil
	}
	return []byte(`"` + time.Time(c).Format("2006-01-02") + `"`), nil
}

func (c *CivilDate) UnmarshalJSON(b []byte) error {
	value := string(b)
	if value == "null" || value == `""` {
		return nil
	}

	value = value[1 : len(value)-1]

	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return err
	}

	*c = CivilDate(t)
	return nil
}

func (c *CivilDate) Scan(value interface{}) error {
	if value == nil {
		*c = CivilDate(time.Time{})
		return nil
	}

	if t, ok := value.(time.Time); ok {
		*c = CivilDate(t)
		return nil
	}

	return fmt.Errorf("cannot scan %T into CivilDate", value)
}

func (c CivilDate) Value() (driver.Value, error) {
	return time.Time(c), nil
}

func (c CivilDate) String() string {
	return time.Time(c).Format("2006-01-02")
}

type UserSearchResult struct {
	ID          int       `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	DOB         CivilDate `json:"dob"`
	Gender      string    `json:"gender"`
	Email       string    `json:"email"`
	CreatedAt   string    `json:"created_at"`
}

type BuddyRequest struct {
	ID          int       `json:"id"`
	RequesterID int       `json:"requester_id"`
	RecipientID int       `json:"recipient_id"`
	Status      string    `json:"status"` // pending, accepted, rejected
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type BuddyRequestWithUser struct {
	ID          int       `json:"id"`
	RequesterID int       `json:"requester_id,omitempty"`
	RecipientID int       `json:"recipient_id,omitempty"`
	UserID      int       `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
