//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procGetLogicalDrives        = kernel32.NewProc("GetLogicalDrives")
	procGetDriveTypeW           = kernel32.NewProc("GetDriveTypeW")
	procGetVolumeInformationW   = kernel32.NewProc("GetVolumeInformationW")
)

const (
	driveRemovable = 2
	driveFixed     = 3
)

type driveState struct {
	Root       string
	Type       uint32
	Serial     uint32
	VolumeName string
	FileSystem string
}

// DetectDefaultRoots indexes all currently accessible local fixed and removable
// volumes. v0.1.28 intentionally includes removable media so swapping a USB
// drive/card does not leave the new volume invisible to automatic indexing.
func DetectDefaultRoots() []string {
	states := DetectIndexableDriveStates()
	roots := make([]string, 0, len(states))
	for _, s := range states {
		roots = append(roots, s.Root)
	}
	if len(roots) == 0 {
		roots = []string{"C:\\"}
	}
	return roots
}

func DetectIndexableDriveStates() []driveState {
	mask, _, _ := procGetLogicalDrives.Call()
	states := make([]driveState, 0, 6)
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := fmt.Sprintf("%c:\\", 'A'+i)
		p, _ := syscall.UTF16PtrFromString(root)
		typ, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(p)))
		if typ != driveFixed && typ != driveRemovable {
			continue
		}
		state, ok := readDriveState(root)
		if !ok {
			// Card-reader drive letters can exist without media inserted.
			// Skip inaccessible removable slots instead of repeatedly indexing them.
			if typ == driveRemovable {
				continue
			}
			// A fixed local volume should normally expose volume information, but
			// keep it indexable if the filesystem is accessible anyway.
			if _, err := os.Stat(root); err != nil {
				continue
			}
			state = driveState{Root: root, Type: uint32(typ)}
		}
		states = append(states, state)
	}
	sort.SliceStable(states, func(i, j int) bool {
		ri := drivePriority(states[i].Type)
		rj := drivePriority(states[j].Type)
		if ri != rj {
			return ri < rj
		}
		return strings.ToLower(states[i].Root) < strings.ToLower(states[j].Root)
	})
	return states
}

func drivePriority(typ uint32) int {
	if typ == driveRemovable {
		return 0
	}
	if typ == driveFixed {
		return 1
	}
	return 2
}

func prioritizeIndexRoots(roots, preferredVolumes []string) []string {
	out := append([]string(nil), roots...)
	preferred := make(map[string]struct{}, len(preferredVolumes))
	for _, p := range preferredVolumes {
		if v := driveVolumeRoot(p); v != "" {
			preferred[strings.ToLower(v)] = struct{}{}
		}
	}
	rank := func(root string) int {
		vol := driveVolumeRoot(root)
		if _, ok := preferred[strings.ToLower(vol)]; ok {
			return 0
		}
		if state, ok := readDriveState(vol); ok {
			return drivePriority(state.Type) + 1
		}
		return 4
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank(out[i]), rank(out[j])
		if ri != rj {
			return ri < rj
		}
		return false
	})
	return out
}

func readDriveState(root string) (driveState, bool) {
	root = driveVolumeRoot(root)
	if root == "" {
		return driveState{}, false
	}
	p, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return driveState{}, false
	}
	typ, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(p)))
	if typ != driveFixed && typ != driveRemovable {
		return driveState{}, false
	}

	var serial uint32
	var maxComponent uint32
	var flags uint32
	volumeName := make([]uint16, 261)
	fileSystem := make([]uint16, 64)
	ok, _, _ := procGetVolumeInformationW.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&volumeName[0])), uintptr(len(volumeName)),
		uintptr(unsafe.Pointer(&serial)),
		uintptr(unsafe.Pointer(&maxComponent)),
		uintptr(unsafe.Pointer(&flags)),
		uintptr(unsafe.Pointer(&fileSystem[0])), uintptr(len(fileSystem)),
	)
	if ok == 0 {
		return driveState{}, false
	}
	return driveState{
		Root:       root,
		Type:       uint32(typ),
		Serial:     serial,
		VolumeName: syscall.UTF16ToString(volumeName),
		FileSystem: syscall.UTF16ToString(fileSystem),
	}, true
}

func driveVolumeRoot(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if len(clean) >= 2 && clean[1] == ':' {
		letter := strings.ToUpper(clean[:1])
		return letter + ":\\"
	}
	return clean
}

// currentDriveStateText returns a stable fingerprint for the volumes that the
// current index configuration points at. The Windows volume serial number is
// the key that lets xFile_search distinguish two different USB drives that both
// happen to receive the same letter such as F:.
func currentDriveStateText(roots []string) string {
	seen := make(map[string]struct{})
	lines := []string{"v1"}
	for _, root := range roots {
		vol := driveVolumeRoot(root)
		if vol == "" {
			continue
		}
		key := strings.ToLower(vol)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if state, ok := readDriveState(vol); ok {
			volume := strings.ReplaceAll(state.VolumeName, "|", "_")
			fs := strings.ReplaceAll(state.FileSystem, "|", "_")
			lines = append(lines, fmt.Sprintf("%s|%d|%08X|%s|%s", state.Root, state.Type, state.Serial, volume, fs))
		} else {
			lines = append(lines, vol+"|MISSING")
		}
	}
	sort.Slice(lines[1:], func(i, j int) bool {
		return strings.ToLower(lines[i+1]) < strings.ToLower(lines[j+1])
	})
	return strings.Join(lines, "\n") + "\n"
}

func writeDriveStateFile(path string, roots []string) error {
	return writeDriveStateText(path, currentDriveStateText(roots))
}

func writeDriveStateText(path, text string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err == nil {
		return nil
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

// driveStateMatches returns (match, known). known=false means this installation
// predates the v0.1.28 fingerprint sidecar and needs one background refresh to
// establish a trustworthy baseline.
func driveStateMatches(path string, roots []string) (bool, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	saved := strings.TrimSpace(strings.ReplaceAll(string(b), "\r\n", "\n"))
	current := strings.TrimSpace(currentDriveStateText(roots))
	return saved == current, true
}

func changedDriveVolumes(path string, roots []string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	saved := driveStateLineMap(string(b))
	current := driveStateLineMap(currentDriveStateText(roots))
	seen := make(map[string]struct{})
	var changed []string
	for _, root := range roots {
		vol := driveVolumeRoot(root)
		if vol == "" {
			continue
		}
		key := strings.ToLower(vol)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if saved[key] != current[key] {
			changed = append(changed, vol)
		}
	}
	return changed
}

func driveStateLineMap(text string) map[string]string {
	out := make(map[string]string)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.EqualFold(line, "v1") {
			continue
		}
		root, _, ok := strings.Cut(line, "|")
		if !ok {
			continue
		}
		root = driveVolumeRoot(root)
		if root != "" {
			out[strings.ToLower(root)] = line
		}
	}
	return out
}
