package model

import (
    "encoding/json"
    "fmt"
    "strings"
    "time"
)

type AutomationTask struct {
    ID            uint       `gorm:"primaryKey" json:"id"`
    CreatedAt     time.Time  `json:"created_at"`
    UpdatedAt     time.Time  `json:"updated_at"`
    Name          string     `json:"name"`
    Enabled       bool       `json:"enabled"`
    ScheduleType  string     `json:"schedule_type"`
    IntervalValue int        `json:"interval_value"`
    IntervalUnit  string     `json:"interval_unit"`
    Weekdays      string     `json:"-"`
    TimeOfDay     string     `json:"time_of_day"`
    SpecificTime  *time.Time `json:"specific_time"`
    LastRunAt     *time.Time `json:"last_run_at"`
    NextRunAt     *time.Time `json:"next_run_at"`
    CreatorID     uint       `json:"creator_id"`
    Status        string     `json:"status"`
    LastMessage   string     `json:"last_message"`
    Steps         []AutomationStep    `gorm:"-" json:"steps"`
}

type AutomationStep struct {
    ID         uint              `gorm:"primaryKey" json:"id"`
    CreatedAt  time.Time         `json:"created_at"`
    UpdatedAt  time.Time         `json:"updated_at"`
    TaskID     uint              `json:"task_id"`
    SortOrder  int               `json:"sort_order"`
    Action     string            `json:"action"`
    Source     string            `json:"source"`
    Target     string            `json:"target"`
    OptionsRaw string            `gorm:"type:text" json:"-"`
    Options    map[string]string `gorm:"-" json:"options"`
}

type AutomationHistory struct {
    ID         uint      `gorm:"primaryKey" json:"id"`
    CreatedAt  time.Time `json:"created_at"`
    TaskID     uint      `gorm:"index" json:"task_id"`
    Status     string    `json:"status"`
    Message    string    `json:"message"`
    StartedAt  time.Time `json:"started_at"`
    FinishedAt time.Time `json:"finished_at"`
}

func (t *AutomationTask) GetWeekdays() []int {
    if t.Weekdays == "" {
        return nil
    }
    parts := strings.Split(t.Weekdays, ",")
    res := make([]int, 0, len(parts))
    for _, p := range parts {
        if p == "" {
            continue
        }
        var v int
        _, err := fmt.Sscanf(p, "%d", &v)
        if err == nil {
            res = append(res, v)
        }
    }
    return res
}

func (t *AutomationTask) SetWeekdays(days []int) {
    if len(days) == 0 {
        t.Weekdays = ""
        return
    }
    strDays := make([]string, 0, len(days))
    for _, d := range days {
        strDays = append(strDays, fmt.Sprintf("%d", d))
    }
    t.Weekdays = strings.Join(strDays, ",")
}

func (s *AutomationStep) SyncOptionsRaw() {
    if len(s.Options) == 0 {
        s.OptionsRaw = ""
        return
    }
    b, err := json.Marshal(s.Options)
    if err != nil {
        s.OptionsRaw = ""
        return
    }
    s.OptionsRaw = string(b)
}

func (s *AutomationStep) LoadOptions() {
    if s.OptionsRaw == "" {
        s.Options = map[string]string{}
        return
    }
    var m map[string]string
    if err := json.Unmarshal([]byte(s.OptionsRaw), &m); err != nil {
        s.Options = map[string]string{}
        return
    }
    s.Options = m
}
