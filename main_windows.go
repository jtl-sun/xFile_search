//go:build windows

package main

import "os"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--indexer" {
		os.Exit(runIndexerChild())
	}
	release, first, err := acquireSingleInstance()
	if err != nil {
		showFatal(err.Error())
		return
	}
	if !first {
		showInfo("xFile_search is already running.")
		return
	}
	defer release()

	app := NewWindowsApp()
	if err := app.Run(); err != nil {
		logf("fatal: %v", err)
		showFatal(err.Error())
	}
}
