package proc

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TieChildrenToMe puts this process in a Job Object that kills every
// process in it when this one ends, however it ends. Processes started
// afterwards join automatically.
func TieChildrenToMe() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return err
	}
	// The handle stays open for the life of the process on purpose.
	return windows.AssignProcessToJobObject(job, windows.CurrentProcess())
}

// bindToLifetime puts the child in a Job Object that kills every process
// in it once l4win exits. Processes the child starts join the job too.
// If the job can't be set up (very old Windows, or a parent job that
// forbids it) the child simply runs without it.
func bindToLifetime(p *os.Process) func() {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return func() {}
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return func() {}
	}

	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.Pid))
	if err != nil {
		windows.CloseHandle(job)
		return func() {}
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		windows.CloseHandle(job)
		return func() {}
	}

	// Closing the handle (or l4win dying, which closes it) ends the job.
	return func() { windows.CloseHandle(job) }
}
