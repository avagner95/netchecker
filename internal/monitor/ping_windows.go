//go:build windows

package monitor

import (
	"context"
	"encoding/binary"
	"net"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// --- WinAPI: ICMP (iphlpapi.dll) ---

var (
	dllIphlpapi       = windows.NewLazySystemDLL("iphlpapi.dll")
	procIcmpCreate    = dllIphlpapi.NewProc("IcmpCreateFile")
	procIcmpClose     = dllIphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho2 = dllIphlpapi.NewProc("IcmpSendEcho2")
)

// IP status codes (subset we care about)
const (
	ipSuccess     = 0
	ipReqTimedOut = 11010
)

// From Windows headers (IP_OPTION_INFORMATION)
type ipOptionInformation struct {
	Ttl         byte
	Tos         byte
	Flags       byte
	OptionsSize byte
	OptionsData *byte
}

// From Windows headers (ICMP_ECHO_REPLY)
type icmpEchoReply struct {
	Address       uint32
	Status        uint32
	RoundTripTime uint32
	DataSize      uint16
	Reserved      uint16
	Data          uintptr
	Options       ipOptionInformation
}

func PingOnce(ctx context.Context, address string, timeoutMs int, payload int) PingOut {
	if timeoutMs <= 0 {
		timeoutMs = 1000
	}
	if payload < 0 {
		payload = 0
	}
	// sanity cap: keep it reasonable (echo payload max is ~65500, but don't go crazy)
	if payload > 65000 {
		payload = 65000
	}

	// Respect cancellation before starting (API call itself is blocking up to timeoutMs)
	if err := ctx.Err(); err != nil {
		return PingOut{OK: false, Err: "fail"}
	}

	// Resolve hostname -> IPv4
	ip, err := resolveIPv4(address)
	if err != nil {
		return PingOut{OK: false, Err: "dns"}
	}

	// Create ICMP handle
	h, _, callErr := procIcmpCreate.Call()
	if h == 0 {
		_ = callErr
		return PingOut{OK: false, Err: "fail"}
	}
	icmpHandle := windows.Handle(h)
	defer func() {
		_, _, _ = procIcmpClose.Call(uintptr(icmpHandle))
	}()

	// Request payload
	var reqData []byte
	if payload > 0 {
		reqData = make([]byte, payload)
	}

	// Reply buffer must be >= sizeof(ICMP_ECHO_REPLY) + request size
	replySize := int(unsafe.Sizeof(icmpEchoReply{})) + maxInt(0, len(reqData)) + 32
	replyBuf := make([]byte, replySize)

	// Timeout for ICMP call
	to := uint32(timeoutMs)

	// Destination IPv4 as uint32 (network order in net.IP; Windows expects IPAddr in ULONG in network order)
	dst := ipToUint32(ip)

	// Call IcmpSendEcho2:
	// DWORD IcmpSendEcho2(
	//   HANDLE IcmpHandle,
	//   HANDLE Event, PIO_APC_ROUTINE ApcRoutine, PVOID ApcContext,
	//   IPAddr DestinationAddress,
	//   LPVOID RequestData, WORD RequestSize,
	//   PIP_OPTION_INFORMATION RequestOptions,
	//   LPVOID ReplyBuffer, DWORD ReplySize,
	//   DWORD Timeout
	// );
	var reqPtr uintptr
	if len(reqData) > 0 {
		reqPtr = uintptr(unsafe.Pointer(&reqData[0]))
	}
	var repPtr uintptr
	if len(replyBuf) > 0 {
		repPtr = uintptr(unsafe.Pointer(&replyBuf[0]))
	}

	// optional: align with darwin style by giving a hard cap slightly above timeout
	// (not strictly needed; IcmpSendEcho2 enforces Timeout)
	_ = time.Duration(timeoutMs+100) * time.Millisecond

	ret, _, _ := procIcmpSendEcho2.Call(
		uintptr(icmpHandle),
		0, 0, 0,
		uintptr(dst),
		reqPtr,
		uintptr(uint16(len(reqData))),
		0,
		repPtr,
		uintptr(uint32(len(replyBuf))),
		uintptr(to),
	)

	// ret == number of replies (0 => failure)
	if ret == 0 {
		// Read status if possible (sometimes buffer still has info; but safest is: timeout/fail)
		// On failure due to timeout, status is usually IP_REQ_TIMED_OUT in reply.
		if len(replyBuf) >= int(unsafe.Sizeof(icmpEchoReply{})) {
			rep := (*icmpEchoReply)(unsafe.Pointer(&replyBuf[0]))
			if rep.Status == ipReqTimedOut {
				return PingOut{OK: false, Err: "timeout"}
			}
		}
		// If context is canceled, treat as fail (your darwin code doesn't return "canceled" for ping)
		if ctx.Err() != nil {
			return PingOut{OK: false, Err: "fail"}
		}
		return PingOut{OK: false, Err: "fail"}
	}

	// Parse first reply
	rep := (*icmpEchoReply)(unsafe.Pointer(&replyBuf[0]))

	if rep.Status != ipSuccess {
		if rep.Status == ipReqTimedOut {
			return PingOut{OK: false, Err: "timeout"}
		}
		return PingOut{OK: false, Err: "fail"}
	}

	ttl := int(rep.Options.Ttl)
	rtt := int(rep.RoundTripTime)

	return PingOut{
		OK:    true,
		TTL:   &ttl,
		RTTms: &rtt,
	}
}

func resolveIPv4(address string) (net.IP, error) {
	// If already IP
	if ip := net.ParseIP(address); ip != nil {
		ip4 := ip.To4()
		if ip4 != nil {
			return ip4, nil
		}
		// IPv6 not supported in this implementation
		return nil, &net.AddrError{Err: "ipv6 not supported", Addr: address}
	}

	// Resolve hostname
	ipa, err := net.ResolveIPAddr("ip4", address)
	if err != nil || ipa == nil || ipa.IP == nil {
		return nil, err
	}
	ip4 := ipa.IP.To4()
	if ip4 == nil {
		return nil, &net.AddrError{Err: "no ipv4", Addr: address}
	}
	return ip4, nil
}

func ipToUint32(ip4 net.IP) uint32 {
	// ip4 must be 4 bytes
	b := ip4.To4()
	return binary.LittleEndian.Uint32(b)
}
