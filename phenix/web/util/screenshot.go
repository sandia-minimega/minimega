package util

import (
	"fmt"
	"time"

	"phenix/api/vm"
	"phenix/web/cache"
)

func GetScreenshot(expName, vmName, size string) ([]byte, error) {
	name := fmt.Sprintf("%s_%s", expName, vmName)

	if screenshot, ok := cache.Get(name); ok {
		return screenshot, nil
	}

	screenshot, err := vm.Screenshot(expName, vmName, size)
	if err != nil {
		return nil, fmt.Errorf("getting screenshot for VM: %w", err)
	}

	if screenshot == nil {
		return nil, fmt.Errorf("VM screenshot not found")
	}

	cache.SetWithExpire(name, screenshot, 10*time.Second)

	return screenshot, nil
}
