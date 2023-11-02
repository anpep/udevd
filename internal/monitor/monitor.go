package monitor

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"syscall"
)

const (
	// groupNone means the socket will not subscribe to any netlink broadcast group.
	groupNone int = 0
	// groupKernel is the netlink broadcast group for packets coming from the kernel.
	// See uevent_net_broadcast() in /lib/kobject_uevent.c
	groupKernel = 1
)

type UeventAction string

const (
	Add    UeventAction = "add"
	Remove UeventAction = "remove"
	Change UeventAction = "change"
)

type Uevent interface {
	fmt.GoStringer
	Action() UeventAction
	DevPath() string
	Attribute(name string) (value string, ok bool)
}

type uevent struct {
	action  UeventAction
	devpath string
	attrs   map[string]string
}

func (u *uevent) Action() UeventAction {
	return u.action
}

func (u *uevent) DevPath() string {
	return u.devpath
}

func (u *uevent) Attribute(name string) (value string, ok bool) {
	value, ok = u.attrs[name]
	return
}

func (u *uevent) GoString() string {
	s := fmt.Sprintf("%v@%v\n", u.action, u.devpath)
	for k, v := range u.attrs {
		s += fmt.Sprintf("%v=%v\n", k, v)
	}
	return s
}

type Monitor struct {
	mu       sync.Mutex
	sockfd   int
	sockaddr syscall.SockaddrNetlink
	handler  UeventHandler
}

type UeventHandler interface {
	HandleUevent(Uevent)
}

func NewMonitor(h UeventHandler) (*Monitor, error) {
	if h == nil {
		return nil, errors.New("cannot create monitor with nil handler")
	}

	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW, syscall.NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return nil, err
	}

	// Address used for binding the monitor socket.
	addr := syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
		Groups: groupKernel,
	}

	// Obtain the PID (port ID) the kernel assigned us.
	// The kernel assigns the process ID to the first socket,
	// but not to subsequent netlink sockets.
	// <https://docs.kernel.org/userspace-api/netlink/intro.html>
	if sa, err := syscall.Getsockname(fd); err != nil {
		return nil, err
	} else if nlsa, ok := sa.(*syscall.SockaddrNetlink); ok {
		addr.Pid = nlsa.Pid
	} else {
		return nil, errors.New("cannot get netlink socket address")
	}

	if err := syscall.Bind(fd, &addr); err != nil {
		return nil, err
	}

	m := &Monitor{
		sockfd:   fd,
		sockaddr: addr,
		handler:  h,
	}
	return m, nil
}

func (m *Monitor) recvUevent() (*uevent, error) {
	// TODO: Handle packets larger than 1K?
	buf := make([]byte, 1024)
	n, from, err := syscall.Recvfrom(m.sockfd, buf, 0)
	if err != nil {
		return nil, err
	}
	if nlsa, ok := from.(*syscall.SockaddrNetlink); !ok {
		return nil, errors.New("cannot get netlink socket address")
	} else if nlsa.Pid != 0 {
		return nil, fmt.Errorf("got a netlink message from a sender other than the kernel (%v)", nlsa.Pid)
	}
	buf = buf[:n]

	u := &uevent{}
	components := strings.Split(strings.TrimRight(string(buf), "\x00"), "\x00")
	if len(components) == 0 {
		return nil, errors.New("cannot parse invalid uevent")
	}

	if split_header := strings.SplitN(components[0], "@", 2); len(split_header) != 2 {
		return nil, errors.New("cannot parse invalid uevent header")
	} else {
		u.action = UeventAction(split_header[0])
		u.devpath = split_header[1]
		u.attrs = make(map[string]string, len(components)-1)
	}

	for _, attr := range components[1:] {
		if split_attr := strings.SplitN(string(attr), "=", 2); len(split_attr) != 2 {
			return nil, errors.New("invalid uevent attribute")
		} else {
			u.attrs[split_attr[0]] = split_attr[1]
		}
	}
	return u, nil
}

func (m *Monitor) Bind() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for {
		u, err := m.recvUevent()
		if err != nil {
			log.Printf("received invalid uevent: %v", err)
		}
		m.handler.HandleUevent(u)
	}
}

func (m *Monitor) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return syscall.Close(m.sockfd)
}
