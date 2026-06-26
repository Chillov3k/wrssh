//go:build pscan

package engine

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	nbnsPort         = 137
	nbnsQuestionType = 0x0021
	nbnsClassIN      = 0x0001
)

type NBInfo struct {
	Hostname string   `json:"hostname,omitempty"`
	Domain   string   `json:"domain,omitempty"`
	MAC      string   `json:"mac,omitempty"`
	Names    []NBName `json:"names,omitempty"`
}

type NBName struct {
	Name   string `json:"name"`
	Suffix string `json:"suffix"`
	Group  bool   `json:"group,omitempty"`
}

func probeNetBIOS(ctx context.Context, host net.IP, timeout time.Duration) (NBInfo, bool) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	host = host.To4()
	if host == nil {
		return NBInfo{}, false
	}

	transactionID := uint16(time.Now().UnixNano())
	request := buildNBNSNodeStatusRequest(transactionID)
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "udp", net.JoinHostPort(host.String(), fmt.Sprint(nbnsPort)))
	if err != nil {
		return NBInfo{}, false
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write(request); err != nil {
		return NBInfo{}, false
	}

	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		return NBInfo{}, false
	}
	info, ok := parseNBNSNodeStatusResponse(buf[:n], transactionID)
	return info, ok
}

func buildNBNSNodeStatusRequest(transactionID uint16) []byte {
	packet := make([]byte, 0, 50)
	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:2], transactionID)
	binary.BigEndian.PutUint16(header[4:6], 1)
	packet = append(packet, header...)
	packet = append(packet, encodedNetBIOSName("*")...)
	question := make([]byte, 4)
	binary.BigEndian.PutUint16(question[0:2], nbnsQuestionType)
	binary.BigEndian.PutUint16(question[2:4], nbnsClassIN)
	packet = append(packet, question...)
	return packet
}

func encodedNetBIOSName(name string) []byte {
	raw := make([]byte, 16)
	for i := range raw {
		raw[i] = ' '
	}
	copy(raw, []byte(name))

	out := make([]byte, 0, 34)
	out = append(out, 32)
	for _, b := range raw {
		out = append(out, 'A'+(b>>4), 'A'+(b&0x0f))
	}
	out = append(out, 0)
	return out
}

func parseNBNSNodeStatusResponse(packet []byte, transactionID uint16) (NBInfo, bool) {
	if len(packet) < 12 || binary.BigEndian.Uint16(packet[0:2]) != transactionID {
		return NBInfo{}, false
	}
	answerCount := int(binary.BigEndian.Uint16(packet[6:8]))
	if answerCount == 0 {
		return NBInfo{}, false
	}

	offset := 12
	next, ok := skipDNSName(packet, offset)
	if !ok || len(packet) < next+4 {
		return NBInfo{}, false
	}
	offset = next + 4

	for i := 0; i < answerCount; i++ {
		next, ok = skipDNSName(packet, offset)
		if !ok || len(packet) < next+10 {
			return NBInfo{}, false
		}
		recordType := binary.BigEndian.Uint16(packet[next : next+2])
		dataLen := int(binary.BigEndian.Uint16(packet[next+8 : next+10]))
		dataStart := next + 10
		dataEnd := dataStart + dataLen
		if dataEnd > len(packet) {
			return NBInfo{}, false
		}
		if recordType == nbnsQuestionType {
			return parseNBNSNodeStatusData(packet[dataStart:dataEnd])
		}
		offset = dataEnd
	}

	return NBInfo{}, false
}

func skipDNSName(packet []byte, offset int) (int, bool) {
	for {
		if offset >= len(packet) {
			return 0, false
		}
		length := int(packet[offset])
		offset++
		if length == 0 {
			return offset, true
		}
		if length&0xc0 == 0xc0 {
			if offset >= len(packet) {
				return 0, false
			}
			return offset + 1, true
		}
		offset += length
		if offset > len(packet) {
			return 0, false
		}
	}
}

func parseNBNSNodeStatusData(data []byte) (NBInfo, bool) {
	if len(data) < 1 {
		return NBInfo{}, false
	}
	nameCount := int(data[0])
	entriesStart := 1
	entriesEnd := entriesStart + nameCount*18
	if nameCount <= 0 || entriesEnd > len(data) {
		return NBInfo{}, false
	}

	info := NBInfo{Names: make([]NBName, 0, nameCount)}
	for offset := entriesStart; offset < entriesEnd; offset += 18 {
		rawName := strings.TrimSpace(string(data[offset : offset+15]))
		suffix := data[offset+15]
		flags := binary.BigEndian.Uint16(data[offset+16 : offset+18])
		if rawName == "" {
			continue
		}

		group := flags&0x8000 != 0
		name := NBName{
			Name:   rawName,
			Suffix: fmt.Sprintf("%02x", suffix),
			Group:  group,
		}
		info.Names = append(info.Names, name)
		if info.Hostname == "" && !group && (suffix == 0x00 || suffix == 0x20) {
			info.Hostname = rawName
		}
		if info.Domain == "" && group && (suffix == 0x00 || suffix == 0x1e) {
			info.Domain = rawName
		}
	}

	stats := data[entriesEnd:]
	if len(stats) >= 6 {
		info.MAC = fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", stats[0], stats[1], stats[2], stats[3], stats[4], stats[5])
	}
	return info, len(info.Names) > 0 || info.Hostname != "" || info.Domain != "" || info.MAC != ""
}
