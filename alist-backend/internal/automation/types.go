package automation

import "time"

const (
	ScheduleModeInterval = "interval"
	ScheduleModeWeekly   = "weekly"
	ScheduleModeOnce     = "once"

	IntervalUnitSecond = "second"
	IntervalUnitMinute = "minute"
	IntervalUnitHour   = "hour"
	IntervalUnitDay    = "day"
)

type ScheduleConfig struct {
	Mode          string `json:"mode"`
	IntervalValue int    `json:"interval_value,omitempty"`
	IntervalUnit  string `json:"interval_unit,omitempty"`
	WeeklyDay     string `json:"weekly_day,omitempty"`
	TimeOfDay     string `json:"time_of_day,omitempty"`
	OnceAt        string `json:"once_at,omitempty"`
}

type StepOptions struct {
	NewName       string `json:"new_name,omitempty"`
	InnerPath     string `json:"inner_path,omitempty"`
	Password      string `json:"password,omitempty"`
	PutIntoNewDir bool   `json:"put_into_new_dir,omitempty"`
	CacheFull     bool   `json:"cache_full,omitempty"`
}

type StepConfig struct {
	Action      string      `json:"action"`
	Source      string      `json:"source"`
	Target      string      `json:"target,omitempty"`
	Description string      `json:"description,omitempty"`
	Options     StepOptions `json:"options"`
}

type JobConfig struct {
	ID       uint           `json:"id,omitempty"`
	Name     string         `json:"name"`
	Enabled  bool           `json:"enabled"`
	Schedule ScheduleConfig `json:"schedule"`
	Steps    []StepConfig   `json:"steps"`
}

type HistoryInfo struct {
	ID         uint       `json:"id"`
	Success    bool       `json:"success"`
	Message    string     `json:"message"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

type JobInfo struct {
	ID        uint           `json:"id"`
	Name      string         `json:"name"`
	Enabled   bool           `json:"enabled"`
	Schedule  ScheduleConfig `json:"schedule"`
	NextRunAt *time.Time     `json:"next_run_at"`
	LastRunAt *time.Time     `json:"last_run_at"`
	CreatorID uint           `json:"creator_id"`
	Steps     []StepConfig   `json:"steps"`
	Histories []HistoryInfo  `json:"histories"`
}
