package op

import (
	"encoding/json"

	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/pkg/errors"
)

func validateNotification(item *model.Notification) error {
	switch item.Type {
	case model.NotificationPushDeer, model.NotificationAzureOAuth:
	default:
		return errors.Errorf("unsupported notification type: %s", item.Type)
	}
	if item.Config != "" && !json.Valid([]byte(item.Config)) {
		return errors.New("notification config must be valid JSON")
	}
	return nil
}

func GetNotificationById(id uint) (*model.Notification, error) {
	return db.GetNotificationById(id)
}

func CreateNotification(item *model.Notification) error {
	if err := validateNotification(item); err != nil {
		return err
	}
	return db.CreateNotification(item)
}

func UpdateNotification(item *model.Notification) error {
	if err := validateNotification(item); err != nil {
		return err
	}
	return db.UpdateNotification(item)
}

func DeleteNotificationById(id uint) error {
	return db.DeleteNotificationById(id)
}

func GetNotifications(pageIndex, pageSize int) (items []model.Notification, count int64, err error) {
	return db.GetNotifications(pageIndex, pageSize)
}
