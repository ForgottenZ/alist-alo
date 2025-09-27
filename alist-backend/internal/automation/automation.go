package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	ScheduleOnce     = "once"
	ScheduleInterval = "interval"
	ScheduleWeekly   = "weekly"
)

type Operation struct {
	Type          string `json:"type"`
	Source        string `json:"source"`
	Destination   string `json:"destination,omitempty"`
	NewName       string `json:"new_name,omitempty"`
	InnerPath     string `json:"inner_path,omitempty"`
	Password      string `json:"password,omitempty"`
	CacheFull     bool   `json:"cache_full,omitempty"`
	PutIntoNewDir bool   `json:"put_into_new_dir,omitempty"`
}

type TaskPayload struct {
	ID            uint        `json:"id"`
	Name          string      `json:"name"`
	Description   string      `json:"description"`
	Enabled       bool        `json:"enabled"`
	ScheduleType  string      `json:"schedule_type"`
	IntervalValue int         `json:"interval_value"`
	IntervalUnit  string      `json:"interval_unit"`
	TimeOfDay     string      `json:"time_of_day"`
	Weekdays      []int       `json:"weekdays"`
	OnceAt        *time.Time  `json:"once_at"`
	Operations    []Operation `json:"operations"`
}

type Job struct {
	Task      *model.AutomationTask
	Ops       []Operation
	Weekdays  []time.Weekday
	running   bool
	manualRun bool
}

type Manager struct {
	mu    sync.Mutex
	tasks map[uint]*Job
	stop  chan struct{}
}

var manager *Manager

func Init() {
	manager = &Manager{
		tasks: map[uint]*Job{},
		stop:  make(chan struct{}),
	}
	var tasks []model.AutomationTask
	if err := db.GetDb().Find(&tasks).Error; err != nil {
		log.Errorf("加载自动化任务失败: %+v", err)
	}
	now := time.Now()
	for i := range tasks {
		ops, err := parseOperations(tasks[i].OperationsJSON)
		if err != nil {
			log.Errorf("解析自动化任务[%d]步骤失败: %+v", tasks[i].ID, err)
			continue
		}
		weekdays := parseWeekdays(tasks[i].WeekdaysJSON)
		job := &Job{
			Task:     &tasks[i],
			Ops:      ops,
			Weekdays: weekdays,
		}
		if tasks[i].Enabled && tasks[i].NextRun == nil {
			next := computeNextRun(now, job)
			if next != nil {
				job.Task.NextRun = next
				_ = db.GetDb().Model(job.Task).Update("next_run", next).Error
			}
		}
		manager.tasks[tasks[i].ID] = job
	}
	go manager.loop()
}

func Shutdown() {
	if manager == nil {
		return
	}
	close(manager.stop)
}

func parseOperations(raw string) ([]Operation, error) {
	if strings.TrimSpace(raw) == "" {
		return []Operation{}, nil
	}
	var ops []Operation
	if err := json.Unmarshal([]byte(raw), &ops); err != nil {
		return nil, err
	}
	return ops, nil
}

func encodeOperations(ops []Operation) (string, error) {
	if len(ops) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(ops)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func parseWeekdays(raw string) []time.Weekday {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var nums []int
	if err := json.Unmarshal([]byte(raw), &nums); err != nil {
		log.Warnf("解析自动化任务周配置失败: %s", err)
		return nil
	}
	res := make([]time.Weekday, 0, len(nums))
	for _, n := range nums {
		if n < 0 || n > 6 {
			continue
		}
		res = append(res, time.Weekday(n))
	}
	sort.Slice(res, func(i, j int) bool { return res[i] < res[j] })
	return res
}

func encodeWeekdays(days []int) (string, error) {
	if len(days) == 0 {
		return "", nil
	}
	b, err := json.Marshal(days)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (m *Manager) loop() {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.dispatch()
		case <-m.stop:
			return
		}
	}
}

func (m *Manager) dispatch() {
	now := time.Now()
	var jobs []*Job
	m.mu.Lock()
	for _, job := range m.tasks {
		if !job.Task.Enabled || job.Task.NextRun == nil {
			continue
		}
		if job.running {
			continue
		}
		if !now.Before(*job.Task.NextRun) {
			job.running = true
			job.manualRun = false
			jobs = append(jobs, job)
		}
	}
	m.mu.Unlock()
	for _, job := range jobs {
		go m.execute(job)
	}
}

func (m *Manager) execute(job *Job) {
	start := time.Now()
	status := "success"
	message := ""
	err := runOperations(job)
	if err != nil {
		status = "error"
		message = err.Error()
		log.Errorf("自动化任务[%d]执行失败: %+v", job.Task.ID, err)
	}
	duration := time.Since(start).Milliseconds()
	m.mu.Lock()
	job.Task.LastRun = &start
	job.Task.LastStatus = status
	if status == "success" {
		job.Task.LastError = ""
	} else {
		job.Task.LastError = message
	}
	next := (*time.Time)(nil)
	if job.Task.Enabled && !job.manualRun {
		next = computeNextRun(start, job)
		job.Task.NextRun = next
		if next == nil && job.Task.ScheduleType == ScheduleOnce {
			job.Task.Enabled = false
		}
	} else {
		job.Task.NextRun = nil
	}
	job.running = false
	m.mu.Unlock()

	updates := map[string]interface{}{
		"last_run":    job.Task.LastRun,
		"last_status": status,
		"last_error":  job.Task.LastError,
	}
	if !job.manualRun {
		updates["next_run"] = job.Task.NextRun
		updates["enabled"] = job.Task.Enabled
	}
	if err := db.GetDb().Model(job.Task).Updates(updates).Error; err != nil {
		log.Errorf("更新自动化任务[%d]状态失败: %+v", job.Task.ID, err)
	}
	history := model.AutomationHistory{
		TaskID:     job.Task.ID,
		ExecutedAt: start,
		Status:     status,
		Message:    message,
		Duration:   duration,
	}
	if err := db.GetDb().Create(&history).Error; err != nil {
		log.Errorf("写入自动化任务历史失败: %+v", err)
	} else {
		trimHistory(job.Task.ID)
	}
}

func computeNextRun(base time.Time, job *Job) *time.Time {
	switch job.Task.ScheduleType {
	case ScheduleInterval:
		duration := parseDuration(job.Task.IntervalValue, job.Task.IntervalUnit)
		if duration <= 0 {
			return nil
		}
		next := base.Add(duration)
		return &next
	case ScheduleWeekly:
		if len(job.Weekdays) == 0 {
			return nil
		}
		hour, min, err := parseTimeOfDay(job.Task.TimeOfDay)
		if err != nil {
			return nil
		}
		candidate := time.Date(base.Year(), base.Month(), base.Day(), hour, min, 0, 0, base.Location())
		for i := 0; i < 14; i++ {
			weekday := candidate.Weekday()
			if !candidate.Before(base) {
				for _, w := range job.Weekdays {
					if weekday == w {
						tmp := candidate
						return &tmp
					}
				}
			}
			candidate = candidate.Add(24 * time.Hour)
		}
		return nil
	case ScheduleOnce:
		if job.Task.OnceAt == nil {
			return nil
		}
		if base.After(*job.Task.OnceAt) {
			return nil
		}
		once := *job.Task.OnceAt
		return &once
	default:
		return nil
	}
}

func parseDuration(value int, unit string) time.Duration {
	if value <= 0 {
		return 0
	}
	switch strings.ToLower(unit) {
	case "s", "sec", "second", "seconds":
		return time.Second * time.Duration(value)
	case "m", "min", "minute", "minutes":
		return time.Minute * time.Duration(value)
	case "h", "hour", "hours":
		return time.Hour * time.Duration(value)
	case "d", "day", "days":
		return 24 * time.Hour * time.Duration(value)
	default:
		return 0
	}
}

func parseTimeOfDay(val string) (int, int, error) {
	if val == "" {
		return 0, 0, fmt.Errorf("empty time")
	}
	t, err := time.Parse("15:04", val)
	if err != nil {
		return 0, 0, err
	}
	return t.Hour(), t.Minute(), nil
}

func runOperations(job *Job) error {
	owner, err := db.GetUserById(job.Task.CreatorID)
	if err != nil {
		return errors.WithMessage(err, "读取任务创建者失败")
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, conf.NoTaskKey, struct{}{})
	ctx = context.WithValue(ctx, "user", owner)
	for idx, op := range job.Ops {
		if err := executeOperation(ctx, owner, op); err != nil {
			return errors.WithMessagef(err, "步骤%d(%s)执行失败", idx+1, op.Type)
		}
	}
	return nil
}

func executeOperation(ctx context.Context, user *model.User, op Operation) error {
	switch op.Type {
	case "copy":
		if op.Source == "" || op.Destination == "" {
			return errors.New("复制操作缺少路径")
		}
		_, err := fs.Copy(ctx, op.Source, op.Destination)
		return err
	case "move":
		if op.Source == "" || op.Destination == "" {
			return errors.New("移动操作缺少路径")
		}
		return fs.Move(ctx, op.Source, op.Destination)
	case "delete":
		if op.Source == "" {
			return errors.New("删除操作缺少路径")
		}
		return fs.Remove(ctx, op.Source)
	case "rename":
		if op.Source == "" || op.NewName == "" {
			return errors.New("重命名操作缺少参数")
		}
		return fs.Rename(ctx, op.Source, op.NewName)
	case "decompress":
		if op.Source == "" || op.Destination == "" {
			return errors.New("解压操作缺少路径")
		}
		inner := op.InnerPath
		if inner == "" {
			inner = "/"
		}
		_, err := fs.ArchiveDecompress(ctx, op.Source, op.Destination, model.ArchiveDecompressArgs{
			ArchiveInnerArgs: model.ArchiveInnerArgs{
				ArchiveArgs: model.ArchiveArgs{
					LinkArgs: model.LinkArgs{Header: http.Header{}},
					Password: op.Password,
				},
				InnerPath: utils.FixAndCleanPath(inner),
			},
			CacheFull:     op.CacheFull,
			PutIntoNewDir: op.PutIntoNewDir,
		})
		return err
	default:
		return errors.Errorf("未知操作类型: %s", op.Type)
	}
}

func trimHistory(taskID uint) {
	var ids []uint
	err := db.GetDb().Model(&model.AutomationHistory{}).
		Where("task_id = ?", taskID).
		Order("executed_at desc, id desc").
		Offset(10).
		Pluck("id", &ids).Error
	if err != nil {
		log.Warnf("读取任务历史失败: %+v", err)
		return
	}
	if len(ids) == 0 {
		return
	}
	if err := db.GetDb().Where("id IN ?", ids).Delete(&model.AutomationHistory{}).Error; err != nil {
		log.Warnf("清理任务历史失败: %+v", err)
	}
}

func (m *Manager) upsert(job *Job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[job.Task.ID] = job
}

func (m *Manager) remove(id uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tasks, id)
}

func (m *Manager) trigger(id uint) error {
	m.mu.Lock()
	job, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return errors.New("任务不存在")
	}
	if job.running {
		m.mu.Unlock()
		return errors.New("任务正在执行")
	}
	job.running = true
	job.manualRun = true
	m.mu.Unlock()
	go m.execute(job)
	return nil
}

func ensureManager() {
	if manager == nil {
		Init()
	}
}

func sanitizePath(user *model.User, path string) (string, error) {
	if path == "" {
		return "", nil
	}
	return user.JoinPath(path)
}

func validatePayload(payload *TaskPayload) error {
	if strings.TrimSpace(payload.Name) == "" {
		return errors.New("任务名称不能为空")
	}
	if len(payload.Operations) == 0 {
		return errors.New("至少需要一个操作步骤")
	}
	switch payload.ScheduleType {
	case ScheduleOnce, ScheduleInterval, ScheduleWeekly:
	default:
		return errors.New("未知的计划类型")
	}
	if payload.ScheduleType == ScheduleInterval && payload.IntervalValue <= 0 {
		return errors.New("间隔任务必须设置大于0的数值")
	}
	if payload.ScheduleType == ScheduleWeekly && len(payload.Weekdays) == 0 {
		return errors.New("每周任务至少选择一天")
	}
	if payload.ScheduleType == ScheduleOnce && payload.OnceAt == nil {
		return errors.New("一次性任务必须设置时间")
	}
	return nil
}

func transformOperations(user *model.User, ops []Operation) ([]Operation, error) {
	res := make([]Operation, len(ops))
	for idx, op := range ops {
		tmp := op
		switch op.Type {
		case "copy", "move":
			src, err := sanitizePath(user, op.Source)
			if err != nil {
				return nil, err
			}
			dst, err := sanitizePath(user, op.Destination)
			if err != nil {
				return nil, err
			}
			tmp.Source = src
			tmp.Destination = dst
			if tmp.Source == "" || tmp.Destination == "" {
				return nil, errors.New("复制/移动操作路径不能为空")
			}
		case "delete":
			src, err := sanitizePath(user, op.Source)
			if err != nil {
				return nil, err
			}
			tmp.Source = src
			if tmp.Source == "" {
				return nil, errors.New("删除操作路径不能为空")
			}
		case "rename":
			src, err := sanitizePath(user, op.Source)
			if err != nil {
				return nil, err
			}
			tmp.Source = src
			if tmp.Source == "" || tmp.NewName == "" {
				return nil, errors.New("重命名参数不能为空")
			}
		case "decompress":
			src, err := sanitizePath(user, op.Source)
			if err != nil {
				return nil, err
			}
			dst, err := sanitizePath(user, op.Destination)
			if err != nil {
				return nil, err
			}
			tmp.Source = src
			tmp.Destination = dst
			if tmp.Source == "" || tmp.Destination == "" {
				return nil, errors.New("解压操作路径不能为空")
			}
			if tmp.InnerPath == "" {
				tmp.InnerPath = "/"
			}
		default:
			return nil, errors.Errorf("未知操作类型: %s", op.Type)
		}
		res[idx] = tmp
	}
	return res, nil
}

func computeInitialNextRun(payload *TaskPayload, enabled bool) *time.Time {
	now := time.Now()
	dummy := model.AutomationTask{
		ScheduleType:  payload.ScheduleType,
		IntervalValue: payload.IntervalValue,
		IntervalUnit:  payload.IntervalUnit,
		TimeOfDay:     payload.TimeOfDay,
		OnceAt:        payload.OnceAt,
	}
	job := &Job{Task: &dummy, Weekdays: toWeekdays(payload.Weekdays)}
	if payload.ScheduleType == ScheduleOnce && payload.OnceAt != nil {
		if payload.OnceAt.Before(now) {
			tmp := now
			return &tmp
		}
	}
	if !enabled {
		return nil
	}
	return computeNextRun(now, job)
}

func toWeekdays(days []int) []time.Weekday {
	res := make([]time.Weekday, 0, len(days))
	for _, n := range days {
		if n >= 0 && n <= 6 {
			res = append(res, time.Weekday(n))
		}
	}
	sort.Slice(res, func(i, j int) bool { return res[i] < res[j] })
	return res
}

func CreateTask(user *model.User, payload *TaskPayload) (*model.AutomationTask, error) {
	ensureManager()
	if err := validatePayload(payload); err != nil {
		return nil, err
	}
	ops, err := transformOperations(user, payload.Operations)
	if err != nil {
		return nil, err
	}
	opsJSON, err := encodeOperations(ops)
	if err != nil {
		return nil, err
	}
	weekdaysJSON, err := encodeWeekdays(payload.Weekdays)
	if err != nil {
		return nil, err
	}
	nextRun := computeInitialNextRun(payload, payload.Enabled)
	task := &model.AutomationTask{
		Name:           payload.Name,
		Description:    payload.Description,
		Enabled:        payload.Enabled,
		ScheduleType:   payload.ScheduleType,
		IntervalValue:  payload.IntervalValue,
		IntervalUnit:   payload.IntervalUnit,
		TimeOfDay:      payload.TimeOfDay,
		WeekdaysJSON:   weekdaysJSON,
		OnceAt:         payload.OnceAt,
		NextRun:        nextRun,
		OperationsJSON: opsJSON,
		CreatorID:      user.ID,
		CreatorName:    user.Username,
	}
	if err := db.GetDb().Create(task).Error; err != nil {
		return nil, err
	}
	job := &Job{Task: task, Ops: ops, Weekdays: toWeekdays(payload.Weekdays)}
	manager.upsert(job)
	return task, nil
}

func UpdateTask(user *model.User, payload *TaskPayload) (*model.AutomationTask, error) {
	ensureManager()
	if payload.ID == 0 {
		return nil, errors.New("任务不存在")
	}
	if err := validatePayload(payload); err != nil {
		return nil, err
	}
	var task model.AutomationTask
	if err := db.GetDb().Where("id = ?", payload.ID).First(&task).Error; err != nil {
		return nil, err
	}
	if !user.IsAdmin() && task.CreatorID != user.ID {
		return nil, errs.PermissionDenied
	}
	ops, err := transformOperations(user, payload.Operations)
	if err != nil {
		return nil, err
	}
	opsJSON, err := encodeOperations(ops)
	if err != nil {
		return nil, err
	}
	weekdaysJSON, err := encodeWeekdays(payload.Weekdays)
	if err != nil {
		return nil, err
	}
	nextRun := computeInitialNextRun(payload, payload.Enabled)
	task.Name = payload.Name
	task.Description = payload.Description
	task.Enabled = payload.Enabled
	task.ScheduleType = payload.ScheduleType
	task.IntervalValue = payload.IntervalValue
	task.IntervalUnit = payload.IntervalUnit
	task.TimeOfDay = payload.TimeOfDay
	task.WeekdaysJSON = weekdaysJSON
	task.OnceAt = payload.OnceAt
	task.OperationsJSON = opsJSON
	task.NextRun = nextRun
	if err := db.GetDb().Save(&task).Error; err != nil {
		return nil, err
	}
	job := &Job{Task: &task, Ops: ops, Weekdays: toWeekdays(payload.Weekdays)}
	manager.upsert(job)
	return &task, nil
}

func DeleteTask(user *model.User, id uint) error {
	ensureManager()
	var task model.AutomationTask
	if err := db.GetDb().Where("id = ?", id).First(&task).Error; err != nil {
		return err
	}
	if !user.IsAdmin() && task.CreatorID != user.ID {
		return errs.PermissionDenied
	}
	if err := db.GetDb().Delete(&task).Error; err != nil {
		return err
	}
	manager.remove(id)
	return nil
}

func ToggleTask(user *model.User, id uint, enabled bool) (*model.AutomationTask, error) {
	ensureManager()
	var task model.AutomationTask
	if err := db.GetDb().Where("id = ?", id).First(&task).Error; err != nil {
		return nil, err
	}
	if !user.IsAdmin() && task.CreatorID != user.ID {
		return nil, errs.PermissionDenied
	}
	task.Enabled = enabled
	payload := TaskPayload{
		ScheduleType:  task.ScheduleType,
		IntervalValue: task.IntervalValue,
		IntervalUnit:  task.IntervalUnit,
		TimeOfDay:     task.TimeOfDay,
		Weekdays:      parseWeekdayInts(task.WeekdaysJSON),
		OnceAt:        task.OnceAt,
		Enabled:       task.Enabled,
	}
	nextRun := computeInitialNextRun(&payload, enabled)
	task.NextRun = nextRun
	if err := db.GetDb().Save(&task).Error; err != nil {
		return nil, err
	}
	job := manager.tasks[id]
	if job != nil {
		job.Task = &task
		job.Weekdays = toWeekdays(payload.Weekdays)
		job.Task.NextRun = nextRun
	}
	return &task, nil
}

func parseWeekdayInts(raw string) []int {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var days []int
	_ = json.Unmarshal([]byte(raw), &days)
	return days
}

func RunNow(user *model.User, id uint) error {
	ensureManager()
	var task model.AutomationTask
	if err := db.GetDb().Where("id = ?", id).First(&task).Error; err != nil {
		return err
	}
	if !user.IsAdmin() && task.CreatorID != user.ID {
		return errs.PermissionDenied
	}
	return manager.trigger(id)
}

func ListTasks(user *model.User) ([]model.AutomationTask, error) {
	ensureManager()
	var tasks []model.AutomationTask
	query := db.GetDb()
	if !user.IsAdmin() {
		query = query.Where("creator_id = ?", user.ID)
	}
	if err := query.Order("created_at desc").Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func GetTaskDetail(user *model.User, id uint) (*model.AutomationTask, []Operation, []model.AutomationHistory, error) {
	ensureManager()
	var task model.AutomationTask
	if err := db.GetDb().Where("id = ?", id).First(&task).Error; err != nil {
		return nil, nil, nil, err
	}
	if !user.IsAdmin() && task.CreatorID != user.ID {
		return nil, nil, nil, errs.PermissionDenied
	}
	ops, err := parseOperations(task.OperationsJSON)
	if err != nil {
		return nil, nil, nil, err
	}
	var histories []model.AutomationHistory
	if err := db.GetDb().Where("task_id = ?", id).Order("executed_at desc, id desc").Limit(10).Find(&histories).Error; err != nil {
		return nil, nil, nil, err
	}
	return &task, ops, histories, nil
}
