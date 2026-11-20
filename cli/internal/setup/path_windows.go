package setup

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	userEnvKey    = `Environment`
	machineEnvKey = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
)

// AddToUserPath puts dir first on the user's PATH (HKCU, no admin needed).
// It reports whether anything changed.
func AddToUserPath(dir string) (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, userEnvKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()

	current, _, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return false, err
	}
	parts := splitPath(current)
	if len(parts) > 0 && samePath(expand(parts[0]), dir) {
		return false, nil // already first
	}
	kept := []string{dir}
	for _, p := range parts {
		if !samePath(expand(p), dir) {
			kept = append(kept, p)
		}
	}
	if err := k.SetExpandStringValue("Path", strings.Join(kept, ";")); err != nil {
		return false, err
	}
	broadcastEnvironmentChange()
	return true, nil
}

// RemoveFromUserPath takes dir off the user's PATH.
func RemoveFromUserPath(dir string) (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, userEnvKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()
	current, _, err := k.GetStringValue("Path")
	if err != nil {
		return false, nil
	}
	parts := splitPath(current)
	var kept []string
	for _, p := range parts {
		if !samePath(expand(p), dir) {
			kept = append(kept, p)
		}
	}
	if len(kept) == len(parts) {
		return false, nil
	}
	if err := k.SetExpandStringValue("Path", strings.Join(kept, ";")); err != nil {
		return false, err
	}
	broadcastEnvironmentChange()
	return true, nil
}

// EffectivePath is the PATH a newly opened terminal gets: the machine PATH
// followed by the user PATH. (Windows puts the machine part first, which
// is why a PHP installed for all users can win over l4win.)
func EffectivePath() string {
	var parts []string
	for _, src := range []struct {
		root registry.Key
		key  string
	}{{registry.LOCAL_MACHINE, machineEnvKey}, {registry.CURRENT_USER, userEnvKey}} {
		k, err := registry.OpenKey(src.root, src.key, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue("Path")
		k.Close()
		if err == nil {
			for _, p := range splitPath(v) {
				parts = append(parts, expand(p))
			}
		}
	}
	if len(parts) == 0 {
		return os.Getenv("PATH")
	}
	return strings.Join(parts, string(filepath.ListSeparator))
}

func splitPath(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ";") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func expand(s string) string {
	if v, err := registry.ExpandString(s); err == nil {
		return v
	}
	return s
}

// broadcastEnvironmentChange tells Explorer (and new terminals it starts)
// to reload the environment, as the System Properties dialog does.
func broadcastEnvironmentChange() {
	user32 := windows.NewLazySystemDLL("user32.dll")
	send := user32.NewProc("SendMessageTimeoutW")
	env, _ := syscall.UTF16PtrFromString("Environment")
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
	)
	var result uintptr
	send.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, uintptr(unsafe.Pointer(&result)))
}
