package automation

import "sync"

var (
    managerOnce sync.Once
    manager     *Manager
)

func Init(loader loadFunc, persist persistFunc) {
    managerOnce.Do(func() {
        manager = NewManager(loader, persist)
        manager.Start()
    })
}

func ManagerInstance() *Manager {
    return manager
}
