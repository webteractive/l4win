package services

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// startDetached starts exe hidden, in its own process group (so Ctrl+C in
// the terminal doesn't reach it) and outside l4win's lifetime, writing its
// output to logFile ("" discards it).
func startDetached(exe string, args []string, logFile string, env []string) (int, error) {
	out := os.DevNull
	if logFile != "" {
		out = logFile
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	start := func(flags uint32) (*exec.Cmd, error) {
		cmd := exec.Command(exe, args...)
		cmd.Dir = filepath.Dir(exe)
		cmd.Stdout, cmd.Stderr = f, f
		if len(env) > 0 {
			cmd.Env = append(os.Environ(), env...)
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP | flags,
		}
		return cmd, cmd.Start()
	}
	// Leave any job the terminal put us in, so closing the terminal doesn't
	// stop the database. Some jobs forbid that; then start it normally.
	cmd, err := start(windows.CREATE_BREAKAWAY_FROM_JOB)
	if err != nil {
		cmd, err = start(0)
	}
	if err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	cmd.Process.Release()
	return pid, nil
}

// processAlive reports whether pid is running and is exe (process IDs get
// reused, so the path is checked too).
func processAlive(pid int, exe string) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if windows.GetExitCodeProcess(h, &code) != nil || code != 259 /* STILL_ACTIVE */ {
		return false
	}
	if exe == "" {
		return true
	}
	return strings.EqualFold(filepath.Clean(imagePath(h)), filepath.Clean(exe))
}

func imagePath(h windows.Handle) string {
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

func killProcess(pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.TerminateProcess(h, 1)
}

// portHolder names the program listening on port, or "" if it's free.
func portHolder(port int) string {
	if !listening(port) {
		return ""
	}
	out, err := exec.Command("netstat", "-ano", "-p", "TCP").Output()
	if err != nil {
		return "another program"
	}
	suffix := ":" + strconv.Itoa(port)
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 5 || f[3] != "LISTENING" || !strings.HasSuffix(f[1], suffix) {
			continue
		}
		pid, _ := strconv.Atoi(f[4])
		if h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)); err == nil {
			p := imagePath(h)
			windows.CloseHandle(h)
			if p != "" {
				return fmt.Sprintf("%s (pid %d)", p, pid)
			}
		}
		// Processes of Windows services hide their path; the name is still listed.
		if name := processName(uint32(pid)); name != "" {
			return fmt.Sprintf("%s (pid %d, probably a Windows service)", name, pid)
		}
		return fmt.Sprintf("process %d", pid)
	}
	return "another program"
}

func processName(pid uint32) string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if e.ProcessID == pid {
			return windows.UTF16ToString(e.ExeFile[:])
		}
	}
	return ""
}
