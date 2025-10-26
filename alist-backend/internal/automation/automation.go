package automation

import (
    "context"
    "errors"
    "sort"
    "sync"
    "time"

    "github.com/alist-org/alist/v3/internal/conf"
    "github.com/alist-org/alist/v3/internal/db"
    "github.com/alist-org/alist/v3/internal/model"
    log "github.com/sirupsen/logrus"
)

type runtimeTask struct {
    Task    *model.AutomationTask
    Steps   []model.AutomationStep
    running bool
}

type Manager struct {
    mu    sync.RWMutex
    tasks map[uint]*runtimeTask
    stop  chan struct{}
    wg    sync.WaitGroup
}

var (
    manager   *Manager
    initOnce  sync.Once
    closeOnce sync.Once
)

func Init() {
    initOnce.Do(func() {
        manager = &Manager{
            tasks: map[uint]*runtimeTask{},
            stop:  make(chan struct{}),
        }
        if err := manager.reload(); err != nil {
            log.Errorf("failed to load automation tasks: %+v", err)
        }
        manager.start()
    })
}

func Close() {
    closeOnce.Do(func() {
        if manager != nil {
            manager.close()
        }
    })
}

func (m *Manager) close() {
    close(m.stop)
    m.wg.Wait()
}

func (m *Manager) reload() error {
    tasks, err := db.ListAutomationTasks()
    if err != nil {
        return err
    }
    now := time.Now()
    m.mu.Lock()
    defer m.mu.Unlock()
    m.tasks = make(map[uint]*runtimeTask, len(tasks))
    for i := range tasks {
        task := &tasks[i]
        steps, err := db.ListAutomationSteps(task.ID)
        if err != nil {
            log.Errorf("failed to load automation steps for task %d: %+v", task.ID, err)
            continue
        }
        sort.SliceStable(steps, func(i, j int) bool {
            if steps[i].SortOrder == steps[j].SortOrder {
                return steps[i].ID < steps[j].ID
            }
            return steps[i].SortOrder < steps[j].SortOrder
        })
        task.Steps = steps
        ensureNextRun(task, now, true)
        m.tasks[task.ID] = &runtimeTask{Task: task, Steps: steps}
    }
    return nil
}

func ensureNextRun(task *model.AutomationTask, from time.Time, persist bool) {
    if !task.Enabled {
        if task.NextRunAt != nil {
            task.NextRunAt = nil
            if persist {
                _ = db.UpdateAutomationTaskFields(task.ID, map[string]interface{}{"next_run_at": nil, "status": "已暂停"})
            }
        }
        return
    }
    if task.ScheduleType == "once" && task.SpecificTime != nil && task.LastRunAt != nil && !task.LastRunAt.Before(*task.SpecificTime) {
        task.NextRunAt = nil
        return
    }
    if task.NextRunAt != nil && task.NextRunAt.After(from) {
        return
    }
    next := computeNextRun(task, from)
    task.NextRunAt = next
    if persist {
        _ = db.UpdateAutomationTaskFields(task.ID, map[string]interface{}{"next_run_at": next, "status": "等待执行"})
    }
}

func (m *Manager) start() {
    m.wg.Add(1)
    go func() {
        defer m.wg.Done()
        ticker := time.NewTicker(time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                m.dispatch()
            case <-m.stop:
                return
            }
        }
    }()
}

func (m *Manager) dispatch() {
    now := time.Now()
    m.mu.RLock()
    pending := make([]*runtimeTask, 0)
    for _, rt := range m.tasks {
        if rt.running {
            continue
        }
        if !rt.Task.Enabled {
            continue
        }
        if rt.Task.NextRunAt == nil {
            continue
        }
        if !rt.Task.NextRunAt.After(now) {
            pending = append(pending, rt)
        }
    }
    m.mu.RUnlock()
    for _, rt := range pending {
        m.launch(rt)
    }
}

func (m *Manager) launch(rt *runtimeTask) {
    m.mu.Lock()
    if rt.running {
        m.mu.Unlock()
        return
    }
    rt.running = true
    m.mu.Unlock()
    m.wg.Add(1)
    go func() {
        defer m.wg.Done()
        executeRuntimeTask(rt)
        m.mu.Lock()
        rt.running = false
        m.mu.Unlock()
    }()
}

func computeNextRun(task *model.AutomationTask, from time.Time) *time.Time {
    switch task.ScheduleType {
    case "interval":
        duration := parseInterval(task.IntervalValue, task.IntervalUnit)
        if duration <= 0 {
            return nil
        }
        next := from.Add(duration)
        return &next
    case "weekly":
        return nextWeeklyRun(task, from)
    case "once":
        if task.SpecificTime == nil {
            return nil
        }
        if task.SpecificTime.Before(from) {
            return nil
        }
        t := task.SpecificTime
        return t
    default:
        return nil
    }
}

func parseInterval(value int, unit string) time.Duration {
    if value <= 0 {
        return 0
    }
    switch unit {
    case "second", "seconds":
        return time.Duration(value) * time.Second
    case "minute", "minutes":
        return time.Duration(value) * time.Minute
    case "hour", "hours":
        return time.Duration(value) * time.Hour
    case "day", "days":
        return time.Duration(value) * 24 * time.Hour
    default:
        return time.Duration(value) * time.Hour
    }
}

func nextWeeklyRun(task *model.AutomationTask, from time.Time) *time.Time {
    weekdays := task.GetWeekdays()
    if len(weekdays) == 0 || task.TimeOfDay == "" {
        return nil
    }
    layoutOptions := []string{"15:04", "15:04:05"}
    var clock time.Time
    var err error
    for _, layout := range layoutOptions {
        clock, err = time.ParseInLocation(layout, task.TimeOfDay, from.Location())
        if err == nil {
            break
        }
    }
    if err != nil {
        return nil
    }
    for i := 0; i < 14; i++ {
        candidate := time.Date(from.Year(), from.Month(), from.Day(), clock.Hour(), clock.Minute(), clock.Second(), 0, from.Location()).AddDate(0, 0, i)
        weekday := int(candidate.Weekday())
        if containsInt(weekdays, weekday) && !candidate.Before(from) {
            return &candidate
        }
    }
    return nil
}

func containsInt(items []int, v int) bool {
    for _, item := range items {
        if item == v {
            return true
        }
    }
    return false
}

func getManager() (*Manager, error) {
    if manager == nil {
        return nil, errors.New("automation manager not initialized")
    }
    return manager, nil
}

func ListTasks() ([]model.AutomationTask, error) {
    tasks, err := db.ListAutomationTasks()
    if err != nil {
        return nil, err
    }
    for i := range tasks {
        steps, err := db.ListAutomationSteps(tasks[i].ID)
        if err != nil {
            return nil, err
        }
        sort.SliceStable(steps, func(i1, j1 int) bool {
            if steps[i1].SortOrder == steps[j1].SortOrder {
                return steps[i1].ID < steps[j1].ID
            }
            return steps[i1].SortOrder < steps[j1].SortOrder
        })
        tasks[i].Steps = steps
    }
    return tasks, nil
}

func GetTask(id uint) (*model.AutomationTask, error) {
    task, err := db.GetAutomationTask(id)
    if err != nil {
        return nil, err
    }
    steps, err := db.ListAutomationSteps(id)
    if err != nil {
        return nil, err
    }
    sort.SliceStable(steps, func(i, j int) bool {
        if steps[i].SortOrder == steps[j].SortOrder {
            return steps[i].ID < steps[j].ID
        }
        return steps[i].SortOrder < steps[j].SortOrder
    })
    task.Steps = steps
    return task, nil
}

func CreateTask(task *model.AutomationTask, steps []model.AutomationStep) (*model.AutomationTask, error) {
    if task.Enabled {
        task.Status = "等待执行"
        task.NextRunAt = computeNextRun(task, time.Now())
    } else {
        task.Status = "已暂停"
        task.NextRunAt = nil
    }
    if err := db.CreateAutomationTask(task, steps); err != nil {
        return nil, err
    }
    rt, err := loadRuntimeTask(task.ID)
    if err != nil {
        return nil, err
    }
    if manager != nil {
        manager.upsert(rt)
    }
    return rt.Task, nil
}

func UpdateTask(task *model.AutomationTask, steps []model.AutomationStep) (*model.AutomationTask, error) {
    existing, err := db.GetAutomationTask(task.ID)
    if err != nil {
        return nil, err
    }
    task.CreatorID = existing.CreatorID
    task.LastRunAt = existing.LastRunAt
    if task.Enabled {
        task.Status = "等待执行"
        task.NextRunAt = computeNextRun(task, time.Now())
    } else {
        task.Status = "已暂停"
        task.NextRunAt = nil
    }
    if err := db.UpdateAutomationTask(task, steps); err != nil {
        return nil, err
    }
    rt, err := loadRuntimeTask(task.ID)
    if err != nil {
        return nil, err
    }
    if manager != nil {
        manager.upsert(rt)
    }
    return rt.Task, nil
}

func DeleteTask(id uint) error {
    if err := db.DeleteAutomationTask(id); err != nil {
        return err
    }
    if manager != nil {
        manager.mu.Lock()
        delete(manager.tasks, id)
        manager.mu.Unlock()
    }
    return nil
}

func ToggleTask(id uint, enabled bool) (*model.AutomationTask, error) {
    rt, err := loadRuntimeTask(id)
    if err != nil {
        return nil, err
    }
    values := map[string]interface{}{"enabled": enabled}
    if enabled {
        rt.Task.Enabled = true
        rt.Task.Status = "等待执行"
        rt.Task.NextRunAt = computeNextRun(rt.Task, time.Now())
        values["status"] = rt.Task.Status
        values["next_run_at"] = rt.Task.NextRunAt
    } else {
        rt.Task.Enabled = false
        rt.Task.Status = "已暂停"
        rt.Task.NextRunAt = nil
        values["status"] = rt.Task.Status
        values["next_run_at"] = nil
    }
    if err := db.UpdateAutomationTaskFields(id, values); err != nil {
        return nil, err
    }
    if manager != nil {
        manager.upsert(rt)
    }
    return rt.Task, nil
}

func RunTaskNow(id uint) error {
    m, err := getManager()
    if err != nil {
        return err
    }
    m.mu.RLock()
    rt, ok := m.tasks[id]
    m.mu.RUnlock()
    if !ok {
        loaded, err := loadRuntimeTask(id)
        if err != nil {
            return err
        }
        m.upsert(loaded)
        rt = loaded
    }
    m.launch(rt)
    return nil
}

func loadRuntimeTask(id uint) (*runtimeTask, error) {
    task, err := db.GetAutomationTask(id)
    if err != nil {
        return nil, err
    }
    steps, err := db.ListAutomationSteps(id)
    if err != nil {
        return nil, err
    }
    sort.SliceStable(steps, func(i, j int) bool {
        if steps[i].SortOrder == steps[j].SortOrder {
            return steps[i].ID < steps[j].ID
        }
        return steps[i].SortOrder < steps[j].SortOrder
    })
    task.Steps = steps
    return &runtimeTask{Task: task, Steps: steps}, nil
}

func (m *Manager) upsert(rt *runtimeTask) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.tasks[rt.Task.ID] = rt
}

func ListHistory(taskID uint) ([]model.AutomationHistory, error) {
    return db.ListAutomationHistory(taskID)
}

func SaveHistory(history *model.AutomationHistory) error {
    return db.CreateAutomationHistory(history)
}

func UpdateTaskStatus(id uint, values map[string]interface{}) {
    if err := db.UpdateAutomationTaskFields(id, values); err != nil {
        log.Errorf("failed to update automation task %d status: %+v", id, err)
    }
}

func BuildBaseContext(user *model.User) context.Context {
    ctx := context.WithValue(context.Background(), "user", user)
    ctx = context.WithValue(ctx, conf.NoTaskKey, true)
    return ctx
}

func UpdateRuntimeAfterRun(rt *runtimeTask, status string, message string, next *time.Time, enabled bool) {
    rt.Task.Status = status
    rt.Task.LastMessage = message
    rt.Task.NextRunAt = next
    rt.Task.Enabled = enabled
}

func RecordHistory(taskID uint, status, message string, start, end time.Time) {
    history := &model.AutomationHistory{
        TaskID:     taskID,
        Status:     status,
        Message:    message,
        StartedAt:  start,
        FinishedAt: end,
    }
    if err := SaveHistory(history); err != nil {
        log.Errorf("failed to write automation history: %+v", err)
    }
}

