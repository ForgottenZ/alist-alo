package model

import "time"

type AutomationTask struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Enabled        bool       `json:"enabled"`
	ScheduleType   string     `json:"schedule_type"`
	IntervalValue  int        `json:"interval_value"`
	IntervalUnit   string     `json:"interval_unit"`
	TimeOfDay      string     `json:"time_of_day"`
	WeekdaysJSON   string     `gorm:"type:text" json:"weekdays_json"`
	OnceAt         *time.Time `json:"once_at"`
	NextRun        *time.Time `json:"next_run"`
	LastRun        *time.Time `json:"last_run"`
	OperationsJSON string     `gorm:"type:text" json:"operations_json"`
	CreatorID      uint       `json:"creator_id"`
	CreatorName    string     `json:"creator_name"`
	LastStatus     string     `json:"last_status"`
	LastError      string     `gorm:"type:text" json:"last_error"`
}

type AutomationHistory struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	TaskID     uint      `json:"task_id"`
	ExecutedAt time.Time `json:"executed_at"`
	Status     string    `json:"status"`
	Message    string    `gorm:"type:text" json:"message"`
	Duration   int64     `json:"duration"`
}
