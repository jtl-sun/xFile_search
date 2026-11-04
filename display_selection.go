package main

import "strings"

// displayMix describes the rows selected for the capped ListView.
// Balanced is true only for a direct recursive directory scope in All mode
// when the full result set is larger than the visible row limit.
type displayMix struct {
	Files    int
	Folders  int
	Balanced bool
}

// selectDisplayIDs keeps the fast full search result intact and only chooses
// which rows are materialized into the ListView. Directory indexes can contain
// very long runs of folder entries before the files beneath them. If the UI
// simply takes ids[:limit], descendant files can look as if they were not found.
//
// For a direct directory scope in All mode, reserve roughly 75% of the visible
// rows for files and 25% for folders, then interleave them 3:1. If one type is
// scarce, the other type fills the unused slots.
func selectDisplayIDs(snap *IndexSnapshot, ids []uint32, limit int, raw string, mode FilterMode) ([]uint32, displayMix) {
	if snap == nil || limit <= 0 || len(ids) == 0 {
		return nil, displayMix{}
	}
	if limit > len(ids) {
		limit = len(ids)
	}

	_, directScope := wholeDirectoryPrefix(strings.TrimSpace(raw))
	if mode != FilterAll || !directScope || len(ids) <= limit {
		out := append([]uint32(nil), ids[:limit]...)
		mix := countDisplayKinds(snap, out)
		return out, mix
	}

	files := make([]uint32, 0, limit)
	folders := make([]uint32, 0, limit)
	for _, id := range ids {
		e, ok := snap.EntryAt(id)
		if !ok {
			continue
		}
		if e.IsDir {
			if len(folders) < limit {
				folders = append(folders, id)
			}
		} else if len(files) < limit {
			files = append(files, id)
		}
		if len(files) >= limit && len(folders) >= limit {
			break
		}
	}

	fileTarget := limit * 3 / 4
	folderTarget := limit - fileTarget
	if fileTarget > len(files) {
		fileTarget = len(files)
	}
	if folderTarget > len(folders) {
		folderTarget = len(folders)
	}
	remaining := limit - fileTarget - folderTarget
	if remaining > 0 {
		extraFiles := len(files) - fileTarget
		if extraFiles > remaining {
			extraFiles = remaining
		}
		fileTarget += extraFiles
		remaining -= extraFiles
	}
	if remaining > 0 {
		extraFolders := len(folders) - folderTarget
		if extraFolders > remaining {
			extraFolders = remaining
		}
		folderTarget += extraFolders
	}

	out := make([]uint32, 0, fileTarget+folderTarget)
	fi, di := 0, 0
	for fi < fileTarget || di < folderTarget {
		for n := 0; n < 3 && fi < fileTarget; n++ {
			out = append(out, files[fi])
			fi++
		}
		if di < folderTarget {
			out = append(out, folders[di])
			di++
		}
		if fi >= fileTarget && di < folderTarget {
			out = append(out, folders[di:folderTarget]...)
			di = folderTarget
		}
	}
	return out, displayMix{Files: fileTarget, Folders: folderTarget, Balanced: true}
}

func countDisplayKinds(snap *IndexSnapshot, ids []uint32) displayMix {
	mix := displayMix{}
	for _, id := range ids {
		e, ok := snap.EntryAt(id)
		if !ok {
			continue
		}
		if e.IsDir {
			mix.Folders++
		} else {
			mix.Files++
		}
	}
	return mix
}
