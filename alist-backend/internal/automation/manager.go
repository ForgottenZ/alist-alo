package automation

import (
"context"
"encoding/json"
"errors"
"fmt"
"path"
"sort"
"strconv"
"strings"
"sync"
"time"

"github.com/alist-org/alist/v3/internal/conf"
"github.com/alist-org/alist/v3/internal/fs"
"github.com/alist-org/alist/v3/internal/model"
"github.com/alist-org/alist/v3/pkg/utils"
)

const (
    operationCopy       = "copy"
    operationMove       = "move"
    operationDelete     = "delete"
    operationRename     = "rename"
    operationDecompress = "decompress"

    scheduleOnce     = "once"
    scheduleInterval = "interval"
    scheduleWeekly   = "weekly"
)

const (
    OperationCopy       = operationCopy
    OperationMove       = operationMove
    OperationDelete     = operationDelete
    OperationRename     = operationRename
    OperationDecompress = operationDecompress

    ScheduleOnce     = scheduleOnce
    ScheduleInterval = scheduleInterval
    ScheduleWeekly   = scheduleWeekly
)

type Operation struct {
    ID           string `json:"id"`
    Type         string `json:"type"`
    Source       string `json:"source"`
    Destination  string `json:"destination,omitempty"`
    NewName      string `json:"new_name,omitempty"`
    Password     string `json:"password,omitempty"`
    InnerPath    string `json:"inner_path,omitempty"`
    CacheFull    *bool  `json:"cache_full,omitempty"`
    PutIntoNewDir *bool `json:"put_into_new_dir,omitempty"`
}

type OperationResult struct {
    Operation Operation `json:"operation"`
    Targets   []string   `json:"targets"`
    Success   bool       `json:"success"`
    Error     string     `json:"error,omitempty"`
}

type History struct {
    ID        string            `json:"id"`
    StartedAt time.Time         `json:"started_at"`
    FinishedAt time.Time        `json:"finished_at"`
    Success   bool              `json:"success"`
    Message   string            `json:"message,omitempty"`
    Results   []OperationResult `json:"results"`
}

type Schedule struct {
    Mode          string     `json:"mode"`
    OnceAt        *time.Time `json:"once_at,omitempty"`
    IntervalValue int        `json:"interval_value,omitempty"`
    IntervalUnit  string     `json:"interval_unit,omitempty"`
    StartAt       *time.Time `json:"start_at,omitempty"`
    Weekdays      []time.Weekday `json:"weekdays,omitempty"`
    TimeOfDay     string     `json:"time_of_day,omitempty"`
}

type Task struct {
    ID        string     `json:"id"`
    Name      string     `json:"name"`
    Enabled   bool       `json:"enabled"`
    Schedule  Schedule   `json:"schedule"`
    Operations []Operation `json:"operations"`
    CreatorID uint       `json:"creator_id"`
    Creator   string     `json:"creator"`
    CreatorRole int      `json:"creator_role"`
    CreatedAt time.Time  `json:"created_at"`
    UpdatedAt time.Time  `json:"updated_at"`
    LastRun   *time.Time `json:"last_run,omitempty"`
    NextRun   *time.Time `json:"next_run,omitempty"`
    LastResult string    `json:"last_result,omitempty"`
    History   []History  `json:"history"`
}

type persistFunc func([]byte) error
type loadFunc func() ([]byte, error)

type Manager struct {
    mu       sync.RWMutex
    tasks    map[string]*Task
    stopCh   chan struct{}
    stopped  chan struct{}
    persist  persistFunc
    loader   loadFunc
    ticker   *time.Ticker
}

func NewManager(loader loadFunc, persist persistFunc) *Manager {
    m := &Manager{
        tasks:   make(map[string]*Task),
        stopCh:  make(chan struct{}),
        stopped: make(chan struct{}),
        persist: persist,
        loader:  loader,
    }
    m.load()
    m.recalculateNextRun(time.Now())
    return m
}

func (m *Manager) Start() {
    m.mu.Lock()
    if m.ticker != nil {
        m.mu.Unlock()
        return
    }
    ticker := time.NewTicker(time.Second * 30)
    m.ticker = ticker
    m.mu.Unlock()
    go func() {
        defer close(m.stopped)
        for {
            select {
            case <-ticker.C:
                m.tick(time.Now())
            case <-m.stopCh:
                ticker.Stop()
                return
            }
        }
    }()
}

func (m *Manager) Stop() {
    m.mu.Lock()
    if m.ticker == nil {
        m.mu.Unlock()
        return
    }
    select {
    case <-m.stopCh:
    default:
        close(m.stopCh)
    }
    m.mu.Unlock()
    <-m.stopped
}

func (m *Manager) load() {
    m.mu.Lock()
    defer m.mu.Unlock()
    if m.loader == nil {
        return
    }
    data, err := m.loader()
    if err != nil {
        return
    }
    if len(data) == 0 {
        return
    }
    var tasks []*Task
    if err := json.Unmarshal(data, &tasks); err != nil {
        return
    }
    for _, t := range tasks {
        task := t
        m.tasks[task.ID] = task
    }
}

func (m *Manager) persistLocked() {
    if m.persist == nil {
        return
    }
    tasks := make([]*Task, 0, len(m.tasks))
    for _, t := range m.tasks {
        tasks = append(tasks, t)
    }
    sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreatedAt.Before(tasks[j].CreatedAt) })
    data, err := json.Marshal(tasks)
    if err != nil {
        return
    }
    _ = m.persist(data)
}

func (m *Manager) recalculateNextRun(now time.Time) {
    for _, t := range m.tasks {
        t.NextRun = calculateNextRun(t, now)
    }
}

func (m *Manager) tick(now time.Time) {
    m.mu.Lock()
    tasks := make([]*Task, 0, len(m.tasks))
    for _, task := range m.tasks {
        tasks = append(tasks, task)
    }
    m.mu.Unlock()
    for _, task := range tasks {
        if shouldRun(task, now) {
            go m.runTask(task.ID, "自动触发")
        }
    }
}

func shouldRun(t *Task, now time.Time) bool {
    if !t.Enabled {
        return false
    }
    if t.NextRun == nil {
        return false
    }
    return !t.NextRun.After(now)
}

func calculateNextRun(t *Task, now time.Time) *time.Time {
    switch t.Schedule.Mode {
    case scheduleOnce:
        if t.Schedule.OnceAt == nil {
            return nil
        }
        if t.LastRun != nil && !t.LastRun.Before(*t.Schedule.OnceAt) {
            return nil
        }
        return t.Schedule.OnceAt
    case scheduleInterval:
        duration := intervalToDuration(t.Schedule.IntervalValue, t.Schedule.IntervalUnit)
        if duration <= 0 {
            return nil
        }
        base := now
        if t.LastRun != nil {
            base = *t.LastRun
        } else if t.Schedule.StartAt != nil {
            base = *t.Schedule.StartAt
            if base.After(now) {
                return &base
            }
        }
        next := base.Add(duration)
        if next.Before(now) {
            diff := now.Sub(base)
            steps := diff/duration + 1
            next = base.Add(duration * steps)
        }
        return &next
    case scheduleWeekly:
        if len(t.Schedule.Weekdays) == 0 {
            return nil
        }
        hour, minute := parseTimeOfDay(t.Schedule.TimeOfDay)
        base := now
        if t.Schedule.StartAt != nil && t.Schedule.StartAt.After(now) {
            base = *t.Schedule.StartAt
        }
        weekdaySet := make(map[time.Weekday]struct{}, len(t.Schedule.Weekdays))
        for _, w := range t.Schedule.Weekdays {
            weekdaySet[w] = struct{}{}
        }
        for i := 0; i < 14; i++ {
            candidate := time.Date(base.Year(), base.Month(), base.Day(), hour, minute, 0, 0, base.Location())
            if candidate.Before(base) {
                candidate = candidate.Add(24 * time.Hour)
            }
            weekday := candidate.Weekday()
            if _, ok := weekdaySet[weekday]; ok {
                if candidate.Before(now) {
                    candidate = candidate.Add(24 * time.Hour)
                    base = candidate
                    continue
                }
                return &candidate
            }
            base = candidate.Add(24 * time.Hour)
        }
        return nil
    default:
        return nil
    }
}

func intervalToDuration(value int, unit string) time.Duration {
    if value <= 0 {
        return 0
    }
    switch unit {
    case "second", "seconds", "sec":
        return time.Duration(value) * time.Second
    case "minute", "minutes", "min":
        return time.Duration(value) * time.Minute
    case "hour", "hours":
        return time.Duration(value) * time.Hour
    case "day", "days":
        return time.Duration(value) * 24 * time.Hour
    default:
        return time.Duration(value) * time.Hour
    }
}

func parseTimeOfDay(value string) (int, int) {
if value == "" {
return 0, 0
}
parts := strings.Split(value, ":")
if len(parts) != 2 {
return 0, 0
}
hour, _ := strconv.Atoi(parts[0])
minute, _ := strconv.Atoi(parts[1])
if hour < 0 || hour > 23 {
hour = 0
}
if minute < 0 || minute > 59 {
minute = 0
}
return hour, minute
}

func (m *Manager) List() []*Task {
    m.mu.RLock()
    defer m.mu.RUnlock()
    tasks := make([]*Task, 0, len(m.tasks))
    for _, task := range m.tasks {
        tasks = append(tasks, cloneTask(task))
    }
    sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreatedAt.Before(tasks[j].CreatedAt) })
    return tasks
}

func cloneTask(t *Task) *Task {
    data, _ := json.Marshal(t)
    var cp Task
    _ = json.Unmarshal(data, &cp)
    return &cp
}

func (m *Manager) Create(t *Task) (*Task, error) {
    if strings.TrimSpace(t.Name) == "" {
        return nil, errors.New("任务名称不能为空")
    }
    t.ID = utils.RandStringRunes(12)
    now := time.Now()
    t.CreatedAt = now
    t.UpdatedAt = now
    t.History = nil
    t.LastResult = ""
    t.LastRun = nil
    t.NextRun = calculateNextRun(t, now)
    m.mu.Lock()
    defer m.mu.Unlock()
    m.tasks[t.ID] = t
    m.persistLocked()
    return cloneTask(t), nil
}

func (m *Manager) Update(t *Task) (*Task, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    origin, ok := m.tasks[t.ID]
    if !ok {
        return nil, errors.New("任务不存在")
    }
    t.CreatedAt = origin.CreatedAt
    if t.CreatorID == 0 {
        t.CreatorID = origin.CreatorID
        t.Creator = origin.Creator
        t.CreatorRole = origin.CreatorRole
    }
    t.History = origin.History
    t.LastRun = origin.LastRun
    t.LastResult = origin.LastResult
    t.UpdatedAt = time.Now()
    t.NextRun = calculateNextRun(t, time.Now())
    m.tasks[t.ID] = t
    m.persistLocked()
    return cloneTask(t), nil
}

func (m *Manager) Delete(id string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    delete(m.tasks, id)
    m.persistLocked()
}

func (m *Manager) Toggle(id string, enabled bool) (*Task, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    t, ok := m.tasks[id]
    if !ok {
        return nil, errors.New("任务不存在")
    }
    t.Enabled = enabled
    t.NextRun = calculateNextRun(t, time.Now())
    t.UpdatedAt = time.Now()
    m.persistLocked()
    return cloneTask(t), nil
}

func (m *Manager) runTask(id string, reason string) (*Task, error) {
    m.mu.RLock()
    task := m.tasks[id]
    m.mu.RUnlock()
    if task == nil {
        return nil, errors.New("任务不存在")
    }
    ctx := context.Background()
    ctx = context.WithValue(ctx, conf.NoTaskKey, true)
    if task.CreatorID != 0 {
        ctx = context.WithValue(ctx, "user", &model.User{ID: task.CreatorID, Username: task.Creator, Role: task.CreatorRole})
    }
    history := History{
        ID:        utils.RandStringRunes(8),
        StartedAt: time.Now(),
    }
    results := make([]OperationResult, 0, len(task.Operations))
    success := true
    for _, op := range task.Operations {
        res := executeOperation(ctx, op)
        results = append(results, res)
        if !res.Success {
            success = false
            break
        }
    }
    history.Results = results
    history.Success = success
    history.FinishedAt = time.Now()
    if success {
        history.Message = fmt.Sprintf("任务 %s 执行成功（%s）", task.Name, reason)
    } else {
        history.Message = fmt.Sprintf("任务 %s 执行失败（%s）", task.Name, reason)
    }

    m.mu.Lock()
    defer m.mu.Unlock()
    cur := m.tasks[id]
    if cur == nil {
        return nil, errors.New("任务不存在")
    }
    cur.LastRun = &history.StartedAt
    if success {
        cur.LastResult = "success"
    } else {
        cur.LastResult = "failed"
    }
    cur.History = append([]History{history}, cur.History...)
    if len(cur.History) > 10 {
        cur.History = cur.History[:10]
    }
    cur.NextRun = calculateNextRun(cur, time.Now())
    m.persistLocked()
    return cloneTask(cur), nil
}

func (m *Manager) RunNow(id string) (*Task, error) {
    return m.runTask(id, "手动执行")
}

func executeOperation(ctx context.Context, op Operation) OperationResult {
    res := OperationResult{
        Operation: op,
    }
    if strings.TrimSpace(op.Source) == "" {
        res.Success = false
        res.Error = "未提供操作路径"
        return res
    }
    switch op.Type {
case operationCopy:
res = executeForTargets(ctx, op, func(target string) error {
dst := op.Destination
if strings.TrimSpace(dst) == "" {
dst = path.Dir(target)
}
_, err := fs.Copy(ctx, target, dst)
return err
})
    case operationMove:
        res = executeForTargets(ctx, op, func(target string) error {
            if strings.TrimSpace(op.Destination) == "" {
                return errors.New("未提供目标路径")
            }
            return fs.Move(ctx, target, op.Destination)
        })
    case operationDelete:
        res = executeForTargets(ctx, op, func(target string) error {
            return fs.Remove(ctx, target)
        })
    case operationRename:
        if hasWildcard(op.Source) {
            res.Success = false
            res.Error = "重命名不支持通配符"
            return res
        }
        if strings.TrimSpace(op.NewName) == "" {
            res.Success = false
            res.Error = "请提供新名称"
            return res
        }
        err := fs.Rename(ctx, op.Source, op.NewName)
        res.Targets = []string{op.Source}
        res.Success = err == nil
        if err != nil {
            res.Error = err.Error()
        }
        return res
case operationDecompress:
res = executeForTargets(ctx, op, func(target string) error {
dst := op.Destination
if strings.TrimSpace(dst) == "" {
dst = path.Dir(target)
}
            cacheFull := true
            if op.CacheFull != nil {
                cacheFull = *op.CacheFull
            }
            putIntoNew := false
            if op.PutIntoNewDir != nil {
                putIntoNew = *op.PutIntoNewDir
            }
            _, err := fs.ArchiveDecompress(ctx, target, dst, model.ArchiveDecompressArgs{
                ArchiveInnerArgs: model.ArchiveInnerArgs{
                    ArchiveArgs: model.ArchiveArgs{
                        Password: op.Password,
                    },
                    InnerPath: utils.FixAndCleanPath(op.InnerPath),
                },
                CacheFull:     cacheFull,
                PutIntoNewDir: putIntoNew,
            })
            return err
        })
    default:
        res.Success = false
        res.Error = "未知操作类型"
    }
    return res
}

func executeForTargets(ctx context.Context, op Operation, fn func(target string) error) OperationResult {
    result := OperationResult{Operation: op}
    targets, err := expandTargets(ctx, op.Source)
    if err != nil {
        result.Success = false
        result.Error = err.Error()
        return result
    }
    if len(targets) == 0 {
        result.Success = true
        result.Targets = []string{}
        return result
    }
    result.Targets = targets
    for _, target := range targets {
        if err := fn(target); err != nil {
            result.Success = false
            result.Error = err.Error()
            return result
        }
    }
    result.Success = true
    return result
}

func expandTargets(ctx context.Context, source string) ([]string, error) {
source = utils.FixAndCleanPath(source)
if !hasWildcard(source) {
return []string{source}, nil
}
dir, pattern := path.Split(source)
dir = utils.FixAndCleanPath(dir)
if dir == "" {
dir = "/"
}
objs, err := fs.List(ctx, dir, &fs.ListArgs{NoLog: true})
if err != nil {
return nil, err
}
matched := make([]string, 0)
for _, obj := range objs {
ok, e := path.Match(pattern, obj.GetName())
if e != nil {
return nil, e
}
if ok {
matched = append(matched, path.Join(dir, obj.GetName()))
}
}
return matched, nil
}

func hasWildcard(v string) bool {
    return strings.ContainsAny(v, "*?[")
}

