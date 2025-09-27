package bootstrap

import (
	"github.com/alist-org/alist/v3/internal/automation"
	log "github.com/sirupsen/logrus"
)

func InitAutomationJobs() {
	if err := automation.Init(); err != nil {
		log.Errorf("failed to initialize automation jobs: %v", err)
	}
}
