package model

import "time"

type AutomationJob struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `json:"name"
                                     gorm:"size:255"`
	Enabled   bool                `json:"enabled"`
	Schedule  string              `gorm:"type:text" json:"schedule"`
	NextRunAt *time.Time          `json:"next_run_at"`
	LastRunAt *time.Time          `json:"last_run_at"`
	CreatorID uint                `json:"creator_id"`
	Steps     []AutomationStep    `json:"steps"`
	Histories []AutomationHistory `json:"histories"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

type AutomationStep struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	JobID     uint   `json:"job_id"`
	SortOrder int    `json:"sort_order"`
	Action    string `json:"action"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	Options   string `gorm:"type:text" json:"options"`
}

type AutomationHistory struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	JobID      uint       `json:"job_id"`
	Success    bool       `json:"success"`
	Message    string     `gorm:"type:text" json:"message"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}
