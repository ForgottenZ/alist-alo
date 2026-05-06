package model

import "time"

const (
	NotificationPushDeer   = "pushdeer"
	NotificationAzureOAuth = "azure_oauth"
)

type Notification struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" binding:"required"`
	Type      string    `json:"type" binding:"required"`
	Enabled   bool      `json:"enabled"`
	Config    string    `json:"config" gorm:"type:text"`
	Remark    string    `json:"remark" gorm:"type:text"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
