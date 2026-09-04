//go:build windows

package main

import (
	"os"
	"os/exec"
)

func restartUpdated(path, update string) error {
	const script = `$update = $args[0]
$target = $args[1]
$clientArgs = @()
if ($args.Length -gt 2) { $clientArgs = $args[2..($args.Length - 1)] }
for ($i = 0; $i -lt 100; $i++) {
	try {
		Move-Item -LiteralPath $update -Destination $target -Force -ErrorAction Stop
		Start-Process -FilePath $target -ArgumentList $clientArgs
		exit 0
	} catch {
		Start-Sleep -Milliseconds 100
	}
}
exit 1`

	args := append([]string{"-NoProfile", "-NonInteractive", "-Command", script, update, path}, os.Args[1:]...)
	if err := exec.Command("powershell.exe", args...).Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
