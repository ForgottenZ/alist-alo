package db

import (
    "time"

    "github.com/alist-org/alist/v3/internal/model"
    "gorm.io/gorm"
)

func ListAutomationTasks() ([]model.AutomationTask, error) {
    var tasks []model.AutomationTask
    if err := db.Order("id asc").Find(&tasks).Error; err != nil {
        return nil, err
    }
    return tasks, nil
}

func GetAutomationTask(id uint) (*model.AutomationTask, error) {
    var task model.AutomationTask
    if err := db.First(&task, id).Error; err != nil {
        return nil, err
    }
    return &task, nil
}

func ListAutomationSteps(taskID uint) ([]model.AutomationStep, error) {
    var steps []model.AutomationStep
    if err := db.Where("task_id = ?", taskID).Order("sort_order asc, id asc").Find(&steps).Error; err != nil {
        return nil, err
    }
    for i := range steps {
        steps[i].LoadOptions()
    }
    return steps, nil
}

func CreateAutomationTask(task *model.AutomationTask, steps []model.AutomationStep) error {
    return db.Transaction(func(tx *gorm.DB) error {
        if err := tx.Create(task).Error; err != nil {
            return err
        }
        if len(steps) > 0 {
            for i := range steps {
                steps[i].TaskID = task.ID
                if steps[i].Options == nil {
                    steps[i].Options = map[string]string{}
                }
                steps[i].SyncOptionsRaw()
            }
            if err := tx.Create(&steps).Error; err != nil {
                return err
            }
        }
        return nil
    })
}

func UpdateAutomationTask(task *model.AutomationTask, steps []model.AutomationStep) error {
    return db.Transaction(func(tx *gorm.DB) error {
        var existing model.AutomationTask
        if err := tx.First(&existing, task.ID).Error; err != nil {
            return err
        }
        task.CreatedAt = existing.CreatedAt
        if err := tx.Save(task).Error; err != nil {
            return err
        }
        if err := tx.Where("task_id = ?", task.ID).Delete(&model.AutomationStep{}).Error; err != nil {
            return err
        }
        if len(steps) > 0 {
            for i := range steps {
                steps[i].TaskID = task.ID
                if steps[i].Options == nil {
                    steps[i].Options = map[string]string{}
                }
                steps[i].SyncOptionsRaw()
            }
            if err := tx.Create(&steps).Error; err != nil {
                return err
            }
        }
        return nil
    })
}

func DeleteAutomationTask(id uint) error {
    return db.Transaction(func(tx *gorm.DB) error {
        if err := tx.Where("task_id = ?", id).Delete(&model.AutomationStep{}).Error; err != nil {
            return err
        }
        if err := tx.Where("task_id = ?", id).Delete(&model.AutomationHistory{}).Error; err != nil {
            return err
        }
        if err := tx.Delete(&model.AutomationTask{}, id).Error; err != nil {
            return err
        }
        return nil
    })
}

func UpdateAutomationTaskFields(id uint, values map[string]interface{}) error {
    if len(values) == 0 {
        return nil
    }
    return db.Model(&model.AutomationTask{}).Where("id = ?", id).Updates(values).Error
}

func CreateAutomationHistory(history *model.AutomationHistory) error {
    return db.Transaction(func(tx *gorm.DB) error {
        if err := tx.Create(history).Error; err != nil {
            return err
        }
        var ids []uint
        if err := tx.Model(&model.AutomationHistory{}).
            Where("task_id = ?", history.TaskID).
            Order("created_at desc, id desc").
            Offset(10).
            Pluck("id", &ids).Error; err != nil {
            return err
        }
        if len(ids) > 0 {
            if err := tx.Where("id IN ?", ids).Delete(&model.AutomationHistory{}).Error; err != nil {
                return err
            }
        }
        return nil
    })
}

func ListAutomationHistory(taskID uint) ([]model.AutomationHistory, error) {
    var history []model.AutomationHistory
    if err := db.Where("task_id = ?", taskID).Order("created_at desc, id desc").Limit(10).Find(&history).Error; err != nil {
        return nil, err
    }
    return history, nil
}

func HasAutomationTask(id uint) (bool, error) {
    var count int64
    if err := db.Model(&model.AutomationTask{}).Where("id = ?", id).Count(&count).Error; err != nil {
        return false, err
    }
    return count > 0, nil
}

func TouchAutomationTaskTimestamp(id uint) error {
    return db.Model(&model.AutomationTask{}).Where("id = ?", id).Update("updated_at", time.Now()).Error
}
