package web

import (
	"fmt"
	"time"

	"gophenix/cache"
	"gophenix/types"
)

func isExperimentLocked(name string) types.Status {
	key := "experiment|" + name

	return cache.Locked(key)
}

func unlockExperiment(name string) {
	key := "experiment|" + name

	cache.Unlock(key)
}

func isVMLocked(exp, name string) types.Status {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	return cache.Locked(key)
}

func unlockVM(exp, name string) {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	cache.Unlock(key)
}

func lockExperimentForCreation(name string) error {
	key := "experiment|" + name

	if status := cache.Lock(key, types.StatusCreating, 5*time.Minute); status != "" {
		return fmt.Errorf("experiment %s is locked with status %s", name, status)
	}

	return nil
}

func lockExperimentForDeletion(name string) error {
	key := "experiment|" + name

	if status := cache.Lock(key, types.StatusDeleting, 1*time.Minute); status != "" {
		return fmt.Errorf("experiment %s is locked with status %s", name, status)
	}

	return nil
}

func lockExperimentForStarting(name string) error {
	key := "experiment|" + name

	if status := cache.Lock(key, types.StatusStarting, 5*time.Minute); status != "" {
		return fmt.Errorf("experiment %s is locked with status %s", name, status)
	}

	return nil
}

func lockExperimentForStopping(name string) error {
	key := "experiment|" + name

	if status := cache.Lock(key, types.StatusStopping, 1*time.Minute); status != "" {
		return fmt.Errorf("experiment %s is locked with status %s", name, status)
	}

	return nil
}

func lockVMForStarting(exp, name string) error {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	if status := cache.Lock(key, types.StatusStarting, 1*time.Minute); status != "" {
		return fmt.Errorf("VM %s is locked with status %s", name, status)
	}

	return nil
}

func lockVMForStopping(exp, name string) error {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	if status := cache.Lock(key, types.StatusStopping, 1*time.Minute); status != "" {
		return fmt.Errorf("VM %s is locked with status %s", name, status)
	}

	return nil
}

func lockVMForRedeploying(exp, name string) error {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	if status := cache.Lock(key, types.StatusRedeploying, 5*time.Minute); status != "" {
		return fmt.Errorf("VM %s is locked with status %s", name, status)
	}

	return nil
}

func lockVMForSnapshotting(exp, name string) error {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	if status := cache.Lock(key, types.StatusSnapshotting, 5*time.Minute); status != "" {
		return fmt.Errorf("VM %s is locked with status %s", name, status)
	}

	return nil
}

func lockVMForRestoring(exp, name string) error {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	if status := cache.Lock(key, types.StatusRestoring, 5*time.Minute); status != "" {
		return fmt.Errorf("VM %s is locked with status %s", name, status)
	}

	return nil
}

func lockVMForCommitting(exp, name string) error {
	key := fmt.Sprintf("vm|%s/%s", exp, name)

	if status := cache.Lock(key, types.StatusCommitting, 5*time.Minute); status != "" {
		return fmt.Errorf("VM %s is locked with status %s", name, status)
	}

	return nil
}
