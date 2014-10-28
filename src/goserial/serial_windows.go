package serial

import (
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

type serialPort struct {
	f  *os.File
	fd syscall.Handle
	rl sync.Mutex
	wl sync.Mutex
	ro *syscall.Overlapped
	wo *syscall.Overlapped
}

type structDCB struct {
	DCBlength, BaudRate                            uint32
	flags                                          [4]byte
	wReserved, XonLim, XoffLim                     uint16
	ByteSize, Parity, StopBits                     byte
	XonChar, XoffChar, ErrorChar, EofChar, EvtChar byte
	wReserved1                                     uint16
}

type structTimeouts struct {
	ReadIntervalTimeout         uint32
	ReadTotalTimeoutMultiplier  uint32
	ReadTotalTimeoutConstant    uint32
	WriteTotalTimeoutMultiplier uint32
	WriteTotalTimeoutConstant   uint32
}

var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")

	nSetCommState        = modkernel32.NewProc("SetCommState")
	nSetCommTimeouts     = modkernel32.NewProc("SetCommTimeouts")
	nSetCommMask         = modkernel32.NewProc("SetCommMask")
	nSetupComm           = modkernel32.NewProc("SetupComm")
	nGetOverlappedResult = modkernel32.NewProc("GetOverlappedResult")
	nCreateEvent         = modkernel32.NewProc("CreateEventW")
	nResetEvent          = modkernel32.NewProc("ResetEvent")
)

func openPort(name string, baud int) (rwc io.ReadWriteCloser, err error) {
	if len(name) > 0 && name[0] != '\\' {
		name = "\\\\.\\" + name
	}

	h, err := syscall.CreateFile(syscall.StringToUTF16Ptr(name),
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL|syscall.FILE_FLAGS_OVERLAPPED,
		0)
	if err != nil {
		return
	}

	f := os.NewFile(h, name)
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	if err = setCommState(h, baud); err != nil {
		return
	}
	if err = setupComm(h, 64, 64); err != nil {
		return
	}
	if err = setCommTimeouts(h); err != nil {
		return
	}
	if err = setCommMask(h); err != nil {
		return
	}

	ro, err := newOverlapped()
	if err != nil {
		return
	}
	wo, err := newOverlapped()
	if err != nil {
		return
	}

	port := serialPort{f: f, fd: h, ro: ro, wo: wo}

	return &port, nil
}

func (p *serialPort) Close() error {
	return p.f.Close()
}

func (p *serialPort) Write(buf []byte) (int, error) {
	p.wl.Lock()
	defer p.wl.Unlock()

	if err := resetEvent(p.wo.HEvent); err != nil {
		return 0, err
	}
	var n uint32
	e := syscall.WriteFile(p.fd, buf, &n, p.wo)
	if e != 0 && e != syscall.ERROR_IO_PENDING {
		return int(n), errno(uintptr(e))
	}
	return getOverlappedResult(p.fd, p.wo)
}

func (p *serialPort) Read(buf []byte) (int, error) {
	if p == nil || p.f == nil {
		return 0, fmt.Errorf("Invalid port on read %v %v", p, p.f)
	}

	p.rl.Lock()
	defer p.rl.Unlock()

	if err := resetEvent(p.ro.HEvent); err != nil {
		return 0, err
	}
	var done uint32
	e := syscall.ReadFile(p.fd, buf, &done, p.ro)
	if e != 0 && e != syscall.ERROR_IO_PENDING {
		return int(done), errno(uintptr(e))
	}
	return getOverlappedResult(p.fd, p.ro)
}

func errno(e uintptr) error {
	if e != 0 {
		return error(e)
	}
	return error(syscall.EINVAL)
}

func setCommState(h syscall.Handle, baud int) error {
	var params structDCB
	params.DCBlength = uint32(unsafe.Sizeof(params))

	params.flags[0] = 0x01  // fBinary
	params.flags[0] |= 0x10 // Assert DSR

	params.BaudRate = uint32(baud)
	params.ByteSize = 8

	r1, _, e1 := nSetCommState.Call(uintptr(h), uintptr(unsafe.Pointer(&params)))
	if r1 == 0 {
		return errno(e1)
	}
	return nil
}

func setCommTimeouts(h syscall.Handle) error {
	var timeouts structTimeouts
	const MAXDWORD = 1<<32 - 1
	timeouts.ReadIntervalTimeout = MAXDWORD
	timeouts.ReadTotalTimeoutMultiplier = MAXDWORD
	timeouts.ReadTotalTimeoutConstant = MAXDWORD - 1

	/* From http://msdn.microsoft.com/en-us/library/aa363190(v=VS.85).aspx

		 For blocking I/O see below:

		 Remarks:

		 If an application sets ReadIntervalTimeout and
		 ReadTotalTimeoutMultiplier to MAXDWORD and sets
		 ReadTotalTimeoutConstant to a value greater than zero and
		 less than MAXDWORD, one of the following occurs when the
		 ReadFile function is called:

		 If there are any bytes in the input buffer, ReadFile returns
		       immediately with the bytes in the buffer.

		 If there are no bytes in the input buffer, ReadFile waits
	               until a byte arrives and then returns immediately.

		 If no bytes arrive within the time specified by
		       ReadTotalTimeoutConstant, ReadFile times out.
	*/

	r1, _, e1 := nSetCommTimeouts.Call(uintptr(h), uintptr(unsafe.Pointer(&timeouts)))
	if r1 == 0 {
		return errno(e1)
	}
	return nil
}

func setupComm(h syscall.Handle, in, out int) error {
	r1, _, e1 := nSetupComm.Call(uintptr(h), uintptr(in), uintptr(out))
	if r1 == 0 {
		return errno(e1)
	}
	return nil
}

func setCommMask(h syscall.Handle) error {
	const EV_RXCHAR = 0x0001
	r1, _, e1 := nSetCommMask.Call(uintptr(h), EV_RXCHAR)
	if r1 == 0 {
		return errno(e1)
	}
	return nil
}

func resetEvent(h syscall.Handle) error {
	r1, _, e1 := nResetEvent.Call(uintptr(h))
	if r1 == 0 {
		return errno(e1)
	}
	return nil
}

func newOverlapped() (*syscall.Overlapped, error) {
	var overlapped syscall.Overlapped
	r1, _, e1 := nCreateEvent.Call(0, 1, 0, 0)
	if r1 == 0 {
		return nil, errno(e1)
	}

	overlapped.HEvent = syscall.Handle(r)
	return &overlapped, nil
}

func getOverlappedResult(h syscall.Handle, overlapped *syscall.Overlapped) (int, error) {
	var n int
	r1, _, e1 := nGetOverlappedResult.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(overlapped)),
		uintptr(unsafe.Pointer(&n)), 1)
	if r1 == 0 {
		return n, errno(e1)
	}

	return n, nil
}
