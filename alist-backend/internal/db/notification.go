package db

import (
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/pkg/errors"
)

func GetNotificationById(id uint) (*model.Notification, error) {
	var item model.Notification
	if err := db.First(&item, id).Error; err != nil {
		return nil, errors.Wrap(err, "failed get notification")
	}
	return &item, nil
}

func CreateNotification(item *model.Notification) error {
	return errors.WithStack(db.Create(item).Error)
}

func UpdateNotification(item *model.Notification) error {
	return errors.WithStack(db.Save(item).Error)
}

func DeleteNotificationById(id uint) error {
	return errors.WithStack(db.Delete(&model.Notification{}, id).Error)
}

func GetNotifications(pageIndex, pageSize int) (items []model.Notification, count int64, err error) {
	notificationDB := db.Model(&model.Notification{})
	if err = notificationDB.Count(&count).Error; err != nil {
		return nil, 0, errors.Wrap(err, "failed get notifications count")
	}
	if err = notificationDB.Order(columnName("id")).Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, errors.Wrap(err, "failed get notifications")
	}
	return items, count, nil
}
