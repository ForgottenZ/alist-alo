package automation

import (
    "context"
    "fmt"
    "strconv"
    "strings"
    stdpath "path"
    "time"

    "github.com/alist-org/alist/v3/internal/db"
    "github.com/alist-org/alist/v3/internal/fs"
    "github.com/alist-org/alist/v3/internal/model"
    "github.com/alist-org/alist/v3/pkg/utils"
    log "github.com/sirupsen/logrus"
)

func executeRuntimeTask(rt *runtimeTask) {
    user := pickUser(rt.Task)
    ctx := BuildBaseContext(user)
    start := time.Now()
    UpdateTaskStatus(rt.Task.ID, map[string]interface{}{
        "status":       "执行中",
        "last_message": "",
        "last_run_at":  start,
    })
    rt.Task.LastRunAt = &start
    status := "执行成功"
    message := "所有步骤已完成"
    success := true
    for _, step := range rt.Steps {
        if err := executeStep(ctx, step); err != nil {
            success = false
            status = "执行失败"
            message = fmt.Sprintf("步骤 %d(%s) 失败: %v", step.SortOrder, step.Action, err)
            log.Errorf("automation task %d step %d failed: %+v", rt.Task.ID, step.ID, err)
            break
        }
    }
    end := time.Now()
    nextRun := rt.Task.NextRunAt
    enabled := rt.Task.Enabled
    if success {
        if rt.Task.ScheduleType == "once" {
            enabled = false
            nextRun = nil
        } else if rt.Task.Enabled {
            nextRun = computeNextRun(rt.Task, end)
        }
    } else {
        if rt.Task.Enabled {
            nextRun = computeNextRun(rt.Task, end)
        }
    }
    if !success && message == "" {
        message = "任务执行失败"
    }
    if success && message == "" {
        message = "任务执行完成"
    }
    UpdateTaskStatus(rt.Task.ID, map[string]interface{}{
        "status":       status,
        "last_message": message,
        "next_run_at":  nextRun,
        "enabled":      enabled,
        "last_run_at":  start,
    })
    UpdateRuntimeAfterRun(rt, status, message, nextRun, enabled)
    RecordHistory(rt.Task.ID, status, message, start, end)
}

func pickUser(task *model.AutomationTask) *model.User {
    if task.CreatorID != 0 {
        if user, err := db.GetUserById(task.CreatorID); err == nil {
            return user
        }
    }
    if admin, err := db.GetUserByRole(model.ADMIN); err == nil {
        return admin
    }
    return &model.User{
        ID:         0,
        Username:  "automation",
        BasePath:  "/",
        Role:      model.ADMIN,
        Permission: -1,
    }
}

func executeStep(ctx context.Context, step model.AutomationStep) error {
    switch step.Action {
    case "copy":
        if step.Target == "" {
            return fmt.Errorf("复制步骤缺少目标目录")
        }
        paths, err := expandPaths(ctx, step.Source)
        if err != nil {
            return err
        }
        for _, p := range paths {
            if _, err := fs.Copy(ctx, p, step.Target); err != nil {
                return err
            }
        }
        return nil
    case "move":
        if step.Target == "" {
            return fmt.Errorf("移动步骤缺少目标目录")
        }
        paths, err := expandPaths(ctx, step.Source)
        if err != nil {
            return err
        }
        for _, p := range paths {
            if err := fs.Move(ctx, p, step.Target); err != nil {
                return err
            }
        }
        return nil
    case "delete":
        paths, err := expandPaths(ctx, step.Source)
        if err != nil {
            return err
        }
        for _, p := range paths {
            if err := fs.Remove(ctx, p); err != nil {
                return err
            }
        }
        return nil
    case "rename":
        if step.Target == "" {
            return fmt.Errorf("重命名步骤缺少新名称")
        }
        paths, err := expandPaths(ctx, step.Source)
        if err != nil {
            return err
        }
        if len(paths) != 1 {
            return fmt.Errorf("重命名步骤只能匹配单个对象")
        }
        return fs.Rename(ctx, paths[0], step.Target)
    case "decompress":
        if step.Target == "" {
            return fmt.Errorf("解压步骤缺少目标目录")
        }
        paths, err := expandPaths(ctx, step.Source)
        if err != nil {
            return err
        }
        password := step.Options["password"]
        inner := step.Options["inner_path"]
        if inner == "" {
            inner = "/"
        } else {
            inner = utils.FixAndCleanPath(inner)
        }
        cacheFull := true
        if v, ok := step.Options["cache_full"]; ok {
            cacheFull = parseBool(v, true)
        }
        putIntoNew := false
        if v, ok := step.Options["put_into_new_dir"]; ok {
            putIntoNew = parseBool(v, false)
        }
        for _, p := range paths {
            _, err := fs.ArchiveDecompress(ctx, p, step.Target, model.ArchiveDecompressArgs{
                ArchiveInnerArgs: model.ArchiveInnerArgs{
                    ArchiveArgs: model.ArchiveArgs{
                        Password: password,
                    },
                    InnerPath:   inner,
                },
                CacheFull:     cacheFull,
                PutIntoNewDir: putIntoNew,
            })
            if err != nil {
                return err
            }
        }
        return nil
    default:
        return fmt.Errorf("未知的步骤动作: %s", step.Action)
    }
}

func expandPaths(ctx context.Context, pattern string) ([]string, error) {
    clean := utils.FixAndCleanPath(pattern)
    if !strings.ContainsAny(clean, "*?[") {
        return []string{clean}, nil
    }
    dir := stdpath.Dir(clean)
    if dir == "." {
        dir = "/"
    }
    base := stdpath.Base(clean)
    objs, err := fs.List(ctx, dir, &fs.ListArgs{Refresh: true, NoLog: true})
    if err != nil {
        return nil, err
    }
    var matched []string
    for _, obj := range objs {
        ok, err := stdpath.Match(base, obj.GetName())
        if err != nil {
            return nil, err
        }
        if ok {
            matched = append(matched, utils.FixAndCleanPath(stdpath.Join(dir, obj.GetName())))
        }
    }
    if len(matched) == 0 {
        return nil, fmt.Errorf("未找到匹配的对象: %s", pattern)
    }
    return matched, nil
}

func parseBool(value string, def bool) bool {
    if value == "" {
        return def
    }
    b, err := strconv.ParseBool(value)
    if err != nil {
        return def
    }
    return b
}
