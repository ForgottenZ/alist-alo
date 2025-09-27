package automation

import (
	"context"
	"encoding/json"
	"fmt"
	stdpath "path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type manager struct {
	mu   sync.Mutex
	jobs map[uint]*jobRuntime
}

type jobRuntime struct {
	job   *jobData
	timer *time.Timer
}

type jobData struct {
	info      JobInfo
	schedule  ScheduleConfig
	creatorID uint
}

var instance = &manager{
	jobs: map[uint]*jobRuntime{},
}

func Init() error {
	return instance.init()
}

func ListJobs() ([]JobInfo, error) {
	return instance.listJobs()
}

func CreateJob(cfg JobConfig, creator *model.User) (JobInfo, error) {
	return instance.createJob(cfg, creator)
}

func UpdateJob(cfg JobConfig, creator *model.User) (JobInfo, error) {
	return instance.updateJob(cfg, creator)
}

func DeleteJob(id uint) error {
	return instance.deleteJob(id)
}

func ToggleJob(id uint, enabled bool) (JobInfo, error) {
	return instance.toggleJob(id, enabled)
}

func RunJobNow(id uint) error {
	return instance.runJobNow(id)
}

func JobHistories(id uint) ([]HistoryInfo, error) {
	return instance.getHistories(id)
}

func (m *manager) init() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs = map[uint]*jobRuntime{}
	tx := db.GetDb().Session(&gorm.Session{})
	var jobs []model.AutomationJob
	err := tx.Preload("Steps", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("sort_order ASC")
	}).Preload("Histories", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("started_at DESC").Limit(10)
	}).Find(&jobs).Error
	if err != nil {
		return err
	}
	now := time.Now()
	for i := range jobs {
		jd, err := m.convertModel(&jobs[i])
		if err != nil {
			log.Errorf("failed to load automation job %d: %v", jobs[i].ID, err)
			continue
		}
		runtime := &jobRuntime{job: jd}
		m.jobs[jd.info.ID] = runtime
		if jd.info.Enabled {
			runtime.schedule(now)
		}
	}
	return nil
}

func (m *manager) listJobs() ([]JobInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	jobs := make([]JobInfo, 0, len(m.jobs))
	for _, runtime := range m.jobs {
		jobs = append(jobs, runtime.job.info)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
	return jobs, nil
}

func (m *manager) createJob(cfg JobConfig, creator *model.User) (JobInfo, error) {
	if err := validateConfig(cfg); err != nil {
		return JobInfo{}, err
	}
	payload, err := json.Marshal(cfg.Schedule)
	if err != nil {
		return JobInfo{}, err
	}
	job := model.AutomationJob{
		Name:      strings.TrimSpace(cfg.Name),
		Enabled:   cfg.Enabled,
		Schedule:  string(payload),
		CreatorID: creator.ID,
	}
	job.Steps, err = buildStepModels(cfg.Steps)
	if err != nil {
		return JobInfo{}, err
	}
	err = db.GetDb().Create(&job).Error
	if err != nil {
		return JobInfo{}, err
	}
	// reload to include histories (empty)
	if err = db.GetDb().Preload("Steps", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("sort_order ASC")
	}).Preload("Histories", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("started_at DESC").Limit(10)
	}).First(&job, job.ID).Error; err != nil {
		return JobInfo{}, err
	}
	jd, err := m.convertModel(&job)
	if err != nil {
		return JobInfo{}, err
	}
	m.mu.Lock()
	m.jobs[jd.info.ID] = &jobRuntime{job: jd}
	if jd.info.Enabled {
		m.jobs[jd.info.ID].schedule(time.Now())
	}
	info := m.jobs[jd.info.ID].job.info
	m.mu.Unlock()
	return info, nil
}

func (m *manager) updateJob(cfg JobConfig, creator *model.User) (JobInfo, error) {
	if cfg.ID == 0 {
		return JobInfo{}, fmt.Errorf("任务不存在")
	}
	if err := validateConfig(cfg); err != nil {
		return JobInfo{}, err
	}
	payload, err := json.Marshal(cfg.Schedule)
	if err != nil {
		return JobInfo{}, err
	}
	var job model.AutomationJob
	tx := db.GetDb().Begin()
	if err = tx.First(&job, cfg.ID).Error; err != nil {
		tx.Rollback()
		return JobInfo{}, err
	}
	if job.CreatorID == 0 {
		job.CreatorID = creator.ID
	}
	job.Name = strings.TrimSpace(cfg.Name)
	job.Enabled = cfg.Enabled
	job.Schedule = string(payload)
	if err = tx.Model(&job).Updates(job).Error; err != nil {
		tx.Rollback()
		return JobInfo{}, err
	}
	if err = tx.Where("job_id = ?", job.ID).Delete(&model.AutomationStep{}).Error; err != nil {
		tx.Rollback()
		return JobInfo{}, err
	}
	job.Steps, err = buildStepModels(cfg.Steps)
	if err != nil {
		tx.Rollback()
		return JobInfo{}, err
	}
	for _, step := range job.Steps {
		if err = tx.Create(&step).Error; err != nil {
			tx.Rollback()
			return JobInfo{}, err
		}
	}
	if err = tx.Commit().Error; err != nil {
		return JobInfo{}, err
	}
	if err = db.GetDb().Preload("Steps", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("sort_order ASC")
	}).Preload("Histories", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("started_at DESC").Limit(10)
	}).First(&job, job.ID).Error; err != nil {
		return JobInfo{}, err
	}
	jd, err := m.convertModel(&job)
	if err != nil {
		return JobInfo{}, err
	}
	m.mu.Lock()
	runtime, ok := m.jobs[jd.info.ID]
	if !ok {
		runtime = &jobRuntime{}
		m.jobs[jd.info.ID] = runtime
	}
	if runtime.timer != nil {
		runtime.timer.Stop()
		runtime.timer = nil
	}
	runtime.job = jd
	if jd.info.Enabled {
		runtime.schedule(time.Now())
	}
	info := runtime.job.info
	m.mu.Unlock()
	return info, nil
}

func (m *manager) deleteJob(id uint) error {
	m.mu.Lock()
	runtime, ok := m.jobs[id]
	if ok && runtime.timer != nil {
		runtime.timer.Stop()
	}
	delete(m.jobs, id)
	m.mu.Unlock()
	return db.GetDb().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("job_id = ?", id).Delete(&model.AutomationStep{}).Error; err != nil {
			return err
		}
		if err := tx.Where("job_id = ?", id).Delete(&model.AutomationHistory{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.AutomationJob{}, id).Error
	})
}

func (m *manager) toggleJob(id uint, enabled bool) (JobInfo, error) {
	m.mu.Lock()
	runtime, ok := m.jobs[id]
	if !ok {
		m.mu.Unlock()
		return JobInfo{}, gorm.ErrRecordNotFound
	}
	runtime.job.info.Enabled = enabled
	runtime.job.info.Schedule = runtime.job.schedule
	runtime.job.info.NextRunAt = nil
	if runtime.timer != nil {
		runtime.timer.Stop()
		runtime.timer = nil
	}
	if enabled {
		runtime.schedule(time.Now())
	}
	info := runtime.job.info
	m.mu.Unlock()
	next := runtime.job.info.NextRunAt
	err := db.GetDb().Model(&model.AutomationJob{}).Where("id = ?", id).Updates(map[string]interface{}{
		"enabled":     enabled,
		"next_run_at": next,
	}).Error
	if err != nil {
		return JobInfo{}, err
	}
	return info, nil
}

func (m *manager) runJobNow(id uint) error {
	m.mu.Lock()
	runtime, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return gorm.ErrRecordNotFound
	}
	go m.execute(id, time.Now())
	return nil
}

func (m *manager) getHistories(id uint) ([]HistoryInfo, error) {
	var histories []model.AutomationHistory
	err := db.GetDb().Where("job_id = ?", id).Order("started_at DESC").Limit(10).Find(&histories).Error
	if err != nil {
		return nil, err
	}
	return convertHistories(histories), nil
}

func (m *manager) convertModel(job *model.AutomationJob) (*jobData, error) {
	var schedule ScheduleConfig
	if len(job.Schedule) > 0 {
		if err := json.Unmarshal([]byte(job.Schedule), &schedule); err != nil {
			return nil, err
		}
	}
	info := JobInfo{
		ID:        job.ID,
		Name:      job.Name,
		Enabled:   job.Enabled,
		Schedule:  schedule,
		NextRunAt: job.NextRunAt,
		LastRunAt: job.LastRunAt,
		CreatorID: job.CreatorID,
		Steps:     make([]StepConfig, 0, len(job.Steps)),
		Histories: convertHistories(job.Histories),
	}
	for _, step := range job.Steps {
		cfg, err := toStepConfig(step)
		if err != nil {
			return nil, err
		}
		info.Steps = append(info.Steps, cfg)
	}
	return &jobData{info: info, schedule: schedule, creatorID: job.CreatorID}, nil
}

func toStepConfig(step model.AutomationStep) (StepConfig, error) {
	cfg := StepConfig{
		Action: step.Action,
		Source: step.Source,
		Target: step.Target,
	}
	if len(step.Options) > 0 {
		if err := json.Unmarshal([]byte(step.Options), &cfg.Options); err != nil {
			return StepConfig{}, err
		}
	}
	if cfg.Options.InnerPath == "" {
		cfg.Options.InnerPath = "/"
	}
	return cfg, nil
}

func convertHistories(histories []model.AutomationHistory) []HistoryInfo {
	ret := make([]HistoryInfo, 0, len(histories))
	for _, h := range histories {
		ret = append(ret, HistoryInfo{
			ID:         h.ID,
			Success:    h.Success,
			Message:    h.Message,
			StartedAt:  h.StartedAt,
			FinishedAt: h.FinishedAt,
		})
	}
	return ret
}

func buildStepModels(steps []StepConfig) ([]model.AutomationStep, error) {
	models := make([]model.AutomationStep, 0, len(steps))
	for idx, step := range steps {
		payload, err := json.Marshal(step.Options)
		if err != nil {
			return nil, err
		}
		models = append(models, model.AutomationStep{
			SortOrder: idx,
			Action:    step.Action,
			Source:    strings.TrimSpace(step.Source),
			Target:    strings.TrimSpace(step.Target),
			Options:   string(payload),
		})
	}
	return models, nil
}

func validateConfig(cfg JobConfig) error {
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("任务名称不能为空")
	}
	if len(cfg.Steps) == 0 {
		return fmt.Errorf("请至少添加一个步骤")
	}
	switch cfg.Schedule.Mode {
	case ScheduleModeInterval:
		if cfg.Schedule.IntervalValue <= 0 {
			return fmt.Errorf("间隔时间必须大于0")
		}
		if parseInterval(cfg.Schedule.IntervalValue, cfg.Schedule.IntervalUnit) <= 0 {
			return fmt.Errorf("请选择有效的间隔单位")
		}
	case ScheduleModeWeekly:
		if cfg.Schedule.WeeklyDay == "" || cfg.Schedule.TimeOfDay == "" {
			return fmt.Errorf("请填写每周执行的时间")
		}
		if _, err := time.Parse("15:04", cfg.Schedule.TimeOfDay); err != nil {
			return fmt.Errorf("每周执行时间格式应为HH:mm")
		}
	case ScheduleModeOnce:
		if cfg.Schedule.OnceAt == "" {
			return fmt.Errorf("请填写指定时间")
		}
		if parseOnce(cfg.Schedule.OnceAt, time.Local) == nil {
			return fmt.Errorf("指定时间格式不正确")
		}
	default:
		return fmt.Errorf("未知的计划模式")
	}
	for _, step := range cfg.Steps {
		if strings.TrimSpace(step.Source) == "" && step.Action != "delete" {
			return fmt.Errorf("步骤源路径不能为空")
		}
		switch step.Action {
		case "copy", "move", "decompress":
			if strings.TrimSpace(step.Target) == "" {
				return fmt.Errorf("步骤目标路径不能为空")
			}
		case "rename":
			if strings.TrimSpace(step.Options.NewName) == "" {
				return fmt.Errorf("重命名的新名称不能为空")
			}
		case "delete":
		default:
			if step.Action == "" {
				return fmt.Errorf("请选择操作类型")
			}
		}
	}
	return nil
}

func (r *jobRuntime) schedule(now time.Time) {
	next := computeNextTime(r.job.schedule, r.job.info.LastRunAt, now)
	r.job.info.NextRunAt = next
	if next == nil {
		_ = db.GetDb().Model(&model.AutomationJob{}).Where("id = ?", r.job.info.ID).Update("next_run_at", nil)
		return
	}
	duration := next.Sub(now)
	if duration < time.Second {
		duration = time.Second
	}
	r.timer = time.AfterFunc(duration, func() {
		instance.execute(r.job.info.ID, time.Now())
	})
	if err := db.GetDb().Model(&model.AutomationJob{}).Where("id = ?", r.job.info.ID).Update("next_run_at", next).Error; err != nil {
		log.Errorf("failed to update next run for job %d: %v", r.job.info.ID, err)
	}
}

func computeNextTime(schedule ScheduleConfig, last *time.Time, now time.Time) *time.Time {
	switch schedule.Mode {
	case ScheduleModeInterval:
		dur := parseInterval(schedule.IntervalValue, schedule.IntervalUnit)
		if dur <= 0 {
			return nil
		}
		base := now
		if last != nil {
			base = last.Add(dur)
			for !base.After(now) {
				base = base.Add(dur)
			}
		} else {
			base = now.Add(dur)
		}
		return &base
	case ScheduleModeWeekly:
		day := parseWeekday(schedule.WeeklyDay)
		if day < 0 {
			return nil
		}
		hh, mm := parseHourMinute(schedule.TimeOfDay)
		target := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, now.Location())
		for target.Weekday() != day || !target.After(now) {
			target = target.Add(24 * time.Hour)
		}
		return &target
	case ScheduleModeOnce:
		t := parseOnce(schedule.OnceAt, now.Location())
		if t == nil || !t.After(now) {
			return nil
		}
		return t
	default:
		return nil
	}
}

func parseInterval(value int, unit string) time.Duration {
	switch unit {
	case IntervalUnitSecond:
		return time.Duration(value) * time.Second
	case IntervalUnitMinute:
		return time.Duration(value) * time.Minute
	case IntervalUnitHour:
		return time.Duration(value) * time.Hour
	case IntervalUnitDay:
		return time.Duration(value) * 24 * time.Hour
	default:
		return 0
	}
}

func parseWeekday(day string) time.Weekday {
	switch strings.ToLower(day) {
	case "sunday", "sun", "0":
		return time.Sunday
	case "monday", "mon", "1":
		return time.Monday
	case "tuesday", "tue", "2":
		return time.Tuesday
	case "wednesday", "wed", "3":
		return time.Wednesday
	case "thursday", "thu", "4":
		return time.Thursday
	case "friday", "fri", "5":
		return time.Friday
	case "saturday", "sat", "6":
		return time.Saturday
	default:
		return -1
	}
}

func parseHourMinute(value string) (int, int) {
	t, err := time.Parse("15:04", value)
	if err != nil {
		return 0, 0
	}
	return t.Hour(), t.Minute()
}

func parseOnce(value string, loc *time.Location) *time.Time {
	if value == "" {
		return nil
	}
	layouts := []string{time.RFC3339, "2006-01-02T15:04", "2006/01/02 15:04", "2006-01-02 15:04"}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return &t
		}
	}
	return nil
}

func (m *manager) execute(id uint, trigger time.Time) {
	m.mu.Lock()
	runtime, ok := m.jobs[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	cfg := runtime.job
	m.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, conf.NoTaskKey, struct{}{})
	if cfg.creatorID != 0 {
		var user model.User
		if err := db.GetDb().First(&user, cfg.creatorID).Error; err == nil {
			ctx = context.WithValue(ctx, "user", &user)
		}
	}
	history := model.AutomationHistory{
		JobID:     id,
		StartedAt: trigger,
		Success:   false,
	}
	if err := db.GetDb().Create(&history).Error; err != nil {
		log.Errorf("failed to record history for job %d: %v", id, err)
	}
	stepErr := runSteps(ctx, cfg.info.Steps)
	finish := time.Now()
	msg := "自动任务执行完成"
	success := true
	if stepErr != nil {
		success = false
		msg = stepErr.Error()
	}
	update := map[string]interface{}{
		"finished_at": finish,
		"success":     success,
		"message":     msg,
	}
	if err := db.GetDb().Model(&model.AutomationHistory{}).Where("id = ?", history.ID).Updates(update).Error; err != nil {
		log.Errorf("failed to update history for job %d: %v", id, err)
	}
	trimHistories(id)
	m.mu.Lock()
	runtime.job.info.LastRunAt = &finish
	if runtime.job.schedule.Mode == ScheduleModeOnce {
		runtime.job.info.Enabled = false
		runtime.job.info.NextRunAt = nil
		if runtime.timer != nil {
			runtime.timer.Stop()
			runtime.timer = nil
		}
		_ = db.GetDb().Model(&model.AutomationJob{}).Where("id = ?", id).Updates(map[string]interface{}{
			"last_run_at": finish,
			"enabled":     false,
			"next_run_at": nil,
		}).Error
	} else {
		runtime.schedule(finish)
		_ = db.GetDb().Model(&model.AutomationJob{}).Where("id = ?", id).Updates(map[string]interface{}{
			"last_run_at": finish,
			"next_run_at": runtime.job.info.NextRunAt,
		}).Error
	}
	if histories, err := m.getHistories(id); err == nil {
		runtime.job.info.Histories = histories
	}
	m.mu.Unlock()
}

func runSteps(ctx context.Context, steps []StepConfig) error {
	for idx, step := range steps {
		if err := executeStep(ctx, step); err != nil {
			return fmt.Errorf("步骤%d失败: %w", idx+1, err)
		}
	}
	return nil
}

func executeStep(ctx context.Context, step StepConfig) error {
	switch step.Action {
	case "copy":
		return handleCopy(ctx, step.Source, step.Target)
	case "move":
		return handleMove(ctx, step.Source, step.Target)
	case "delete":
		return handleDelete(ctx, step.Source)
	case "rename":
		return handleRename(ctx, step.Source, step.Options.NewName)
	case "decompress":
		return handleDecompress(ctx, step)
	default:
		return fmt.Errorf("未知的操作类型 %s", step.Action)
	}
}

func handleCopy(ctx context.Context, srcPattern, dst string) error {
	targets, err := expandTargets(ctx, srcPattern)
	if err != nil {
		return err
	}
	dst = utils.FixAndCleanPath(dst)
	for _, target := range targets {
		if err := copyRecursive(ctx, target, dst); err != nil {
			return err
		}
	}
	return nil
}

func handleMove(ctx context.Context, srcPattern, dst string) error {
	targets, err := expandTargets(ctx, srcPattern)
	if err != nil {
		return err
	}
	dst = utils.FixAndCleanPath(dst)
	for _, target := range targets {
		if err := fs.Move(ctx, target, dst); err != nil {
			return err
		}
	}
	return nil
}

func handleDelete(ctx context.Context, pattern string) error {
	targets, err := expandTargets(ctx, pattern)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := fs.Remove(ctx, target); err != nil {
			return err
		}
	}
	return nil
}

func handleRename(ctx context.Context, pattern, newName string) error {
	targets, err := expandTargets(ctx, pattern)
	if err != nil {
		return err
	}
	if len(targets) != 1 {
		return fmt.Errorf("重命名仅支持单个目标")
	}
	return fs.Rename(ctx, targets[0], newName)
}

func handleDecompress(ctx context.Context, step StepConfig) error {
	targets, err := expandTargets(ctx, step.Source)
	if err != nil {
		return err
	}
	dst := utils.FixAndCleanPath(step.Target)
	for _, target := range targets {
		if _, err := fs.ArchiveDecompress(ctx, target, dst, model.ArchiveDecompressArgs{
			ArchiveInnerArgs: model.ArchiveInnerArgs{
				ArchiveArgs: model.ArchiveArgs{Password: step.Options.Password},
				InnerPath:   utils.FixAndCleanPath(step.Options.InnerPath),
			},
			CacheFull:     step.Options.CacheFull,
			PutIntoNewDir: step.Options.PutIntoNewDir,
		}); err != nil {
			return err
		}
	}
	return nil
}

func expandTargets(ctx context.Context, pattern string) ([]string, error) {
	cleaned := utils.FixAndCleanPath(pattern)
	if !strings.ContainsAny(cleaned, "*?[") {
		return []string{cleaned}, nil
	}
	dir := stdpath.Dir(cleaned)
	base := stdpath.Base(cleaned)
	objs, err := fs.List(ctx, dir, &fs.ListArgs{Refresh: true, NoLog: true})
	if err != nil {
		return nil, err
	}
	results := make([]string, 0)
	for _, obj := range objs {
		matched, err := stdpath.Match(base, obj.GetName())
		if err != nil {
			return nil, err
		}
		if matched {
			results = append(results, stdpath.Join(dir, obj.GetName()))
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("未找到匹配项: %s", pattern)
	}
	return results, nil
}

func copyRecursive(ctx context.Context, src, dst string) error {
	dst = utils.FixAndCleanPath(dst)
	obj, err := fs.Get(ctx, src, &fs.GetArgs{})
	if err != nil {
		return err
	}
	if obj.IsDir() {
		targetDir := stdpath.Join(dst, obj.GetName())
		if _, err := fs.Get(ctx, targetDir, &fs.GetArgs{NoLog: true}); err != nil {
			_ = fs.MakeDir(ctx, targetDir)
		}
		children, err := fs.List(ctx, src, &fs.ListArgs{Refresh: true})
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := copyRecursive(ctx, stdpath.Join(src, child.GetName()), targetDir); err != nil {
				return err
			}
		}
		return nil
	}
	_, err = fs.Copy(ctx, src, dst)
	return err
}

func trimHistories(jobID uint) {
	dbx := db.GetDb()
	var ids []uint
	if err := dbx.Model(&model.AutomationHistory{}).Where("job_id = ?", jobID).Order("started_at DESC").Offset(10).Pluck("id", &ids).Error; err != nil {
		return
	}
	if len(ids) > 0 {
		_ = dbx.Where("id IN ?", ids).Delete(&model.AutomationHistory{}).Error
	}
}
