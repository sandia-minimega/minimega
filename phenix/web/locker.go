package web

import (
	"fmt"
	"time"

	"phenix/web/cache"
)

func isExperimentLocked(name string) cache.Status {
	key := "experiment|" + name

	return cache.Locked(key)
}

func unlockExperiment(name string) {
	key := "experiment|" + name

	cache.Unlock(key)
}

func isVMLocked(exp, name string) cache.Status {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	return cache.Locked(key)
}

func unlockVM(exp, name string) {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	cache.Unlock(key)
}

func lockExperimentForCreation(name string) error {
	key := "experiment|" + name

	if status := cache.Lock(key, cache.StatusCreating, 5*time.Minute); status != "" {
		return fmt.Errorf("experiment %s is locked with status %s", name, status)
	}

	return nil
}

func lockExperimentForDeletion(name string) error {
	key := "experiment|" + name

	if status := cache.Lock(key, cache.StatusDeleting, 1*time.Minute); status != "" {
		return fmt.Errorf("experiment %s is locked with status %s", name, status)
	}

	return nil
}

func lockExperimentForStarting(name string) error {
	key := "experiment|" + name

	if status := cache.Lock(key, cache.StatusStarting, 5*time.Minute); status != "" {
		return fmt.Errorf("experiment %s is locked with status %s", name, status)
	}

	return nil
}

func lockExperimentForStopping(name string) error {
	key := "experiment|" + name

	if status := cache.Lock(key, cache.StatusStopping, 1*time.Minute); status != "" {
		return fmt.Errorf("experiment %s is locked with status %s", name, status)
	}

	return nil
}

func lockVMForStarting(exp, name string) error {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	if status := cache.Lock(key, cache.StatusStarting, 1*time.Minute); status != "" {
		return fmt.Errorf("VM %s is locked with status %s", name, status)
	}

	return nil
}

func lockVMForStopping(exp, name string) error {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	if status := cache.Lock(key, cache.StatusStopping, 1*time.Minute); status != "" {
		return fmt.Errorf("VM %s is locked with status %s", name, status)
	}

	return nil
}

func lockVMForRedeploying(exp, name string) error {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	if status := cache.Lock(key, cache.StatusRedeploying, 5*time.Minute); status != "" {
		return fmt.Errorf("VM %s is locked with status %s", name, status)
	}

	return nil
}

func lockVMForSnapshotting(exp, name string) error {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	if status := cache.Lock(key, cache.StatusSnapshotting, 5*time.Minute); status != "" {
		return fmt.Errorf("VM %s is locked with status %s", name, status)
	}

	return nil
}

func lockVMForRestoring(exp, name string) error {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	if status := cache.Lock(key, cache.StatusRestoring, 5*time.Minute); status != "" {
		return fmt.Errorf("VM %s is locked with status %s", name, status)
	}

	return nil
}

func lockVMForCommitting(exp, name string) error {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	if status := cache.Lock(key, cache.StatusCommitting, 5*time.Minute); status != "" {
		return fmt.Errorf("VM %s is locked with status %s", name, status)
	}

	return nil
}
