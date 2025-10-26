package bootstrap

import "github.com/alist-org/alist/v3/internal/automation"

func InitAutomation() {
    automation.Init()
}

func CloseAutomation() {
    automation.Close()
}
