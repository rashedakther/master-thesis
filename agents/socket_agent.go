package agents

import (
	. "github.com/mpiraux/master-thesis"
	"github.com/dustin/go-broadcast"
	"syscall"
	"unsafe"
)

type ECNStatus int

const (
	ECNStatusNonECT ECNStatus = 0
	ECNStatusECT_1            = 1
	ECNStatusECT_0            = 2
	ECNStatusCE               = 3
)

type SocketAgent struct {
	BaseAgent
	conn              *Connection
	ecn               bool
	TotalDataReceived int
	DatagramsReceived int
	SocketStatus      broadcast.Broadcaster //type: err
	ECNStatus         broadcast.Broadcaster //type: ECNStatus
}

func (a *SocketAgent) Run(conn *Connection) {
	a.Init("SocketAgent", conn.SourceCID)
	a.conn = conn
	a.SocketStatus = broadcast.NewBroadcaster(10)
	a.ECNStatus = broadcast.NewBroadcaster(1000)
	recChan := make(chan []byte)

	go func() {
		for {
			recBuf := make([]byte, MaxUDPPayloadSize)
			i, err := conn.UdpConnection.Read(recBuf)
			if err != nil {
				a.Logger.Println("Closing UDP socket because of error", err.Error())
				close(recChan)
				a.SocketStatus.Submit(err)
				break
			}

			if a.ecn {
				s, err := a.conn.UdpConnection.SyscallConn()
				if err != nil {
					a.Logger.Println("Error when retrieving ECN status", err.Error())
					break
				}
				var ecn uint
				f := func(fd uintptr) {
					syscall.Syscall6(syscall.SYS_GETSOCKOPT, fd, uintptr(syscall.IPPROTO_IP), uintptr(syscall.IP_TOS), uintptr(unsafe.Pointer(&ecn)), uintptr(unsafe.Sizeof(ecn)), 0)
				}
				err = s.Control(f)
				if err != nil {
					a.Logger.Println("Error when retrieving ECN status", err.Error())
					break
				}
				a.Logger.Println("ECN value received", ecn & 0x03)
				a.ECNStatus.Submit(ECNStatus(ecn & 0x03))
			}

			a.TotalDataReceived += i
			a.DatagramsReceived += 1
			a.Logger.Printf("Received %d bytes from UDP socket\n", i)
			payload := make([]byte, i)
			copy(payload, recBuf[:i])
			recChan <- payload
		}
	}()

	go func() {
		defer a.Logger.Println("Agent terminated")
		defer close(a.closed)
		for {
			select {
			case p, open := <-recChan:
				if !open {
					return
				}

				conn.IncomingPayloads.Submit(p)
			case <-a.close:
				conn.UdpConnection.Close()
				// TODO: Close this agent gracefully
				return
			}
		}
	}()
}

func (a *SocketAgent) ConfigureECN() error {
	s, err := a.conn.UdpConnection.SyscallConn()
	if err != nil {
		return err
	}
	f := func(fd uintptr) {
		err = syscall.SetsockoptByte(int(fd), syscall.IPPROTO_IP, syscall.IP_RECVTOS, 1)
		if err != nil {
			a.ecn = err == nil
		}
		err = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_TOS, 2) //INET_ECN_ECT_0
		a.ecn = err == nil
	}
	return s.Control(f)
}
