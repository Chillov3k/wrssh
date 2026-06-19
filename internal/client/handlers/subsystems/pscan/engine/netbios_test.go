//go:build pscan

package engine

import (
	"encoding/binary"
	"testing"
)

func TestParseNBNSNodeStatusResponse(t *testing.T) {
	const transactionID uint16 = 0x1234

	question := append(encodedNetBIOSName("*"), 0, nbnsQuestionType, 0, nbnsClassIN)
	data := []byte{3}
	data = append(data, nbNameEntry("WINHOST", 0x00, false)...)
	data = append(data, nbNameEntry("LABDOMAIN", 0x00, true)...)
	data = append(data, nbNameEntry("WINHOST", 0x20, false)...)
	data = append(data, []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}...)

	response := make([]byte, 12)
	binary.BigEndian.PutUint16(response[0:2], transactionID)
	binary.BigEndian.PutUint16(response[2:4], 0x8500)
	binary.BigEndian.PutUint16(response[4:6], 1)
	binary.BigEndian.PutUint16(response[6:8], 1)
	response = append(response, question...)
	response = append(response, 0xc0, 0x0c)
	response = append(response, 0, nbnsQuestionType, 0, nbnsClassIN, 0, 0, 0, 0)
	rdLength := make([]byte, 2)
	binary.BigEndian.PutUint16(rdLength, uint16(len(data)))
	response = append(response, rdLength...)
	response = append(response, data...)

	info, ok := parseNBNSNodeStatusResponse(response, transactionID)
	if !ok {
		t.Fatalf("parseNBNSNodeStatusResponse returned !ok")
	}
	if info.Hostname != "WINHOST" {
		t.Fatalf("hostname = %q, want WINHOST", info.Hostname)
	}
	if info.Domain != "LABDOMAIN" {
		t.Fatalf("domain = %q, want LABDOMAIN", info.Domain)
	}
	if info.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("mac = %q, want aa:bb:cc:dd:ee:ff", info.MAC)
	}
	if len(info.Names) != 3 {
		t.Fatalf("names = %+v, want 3 names", info.Names)
	}
}

func nbNameEntry(name string, suffix byte, group bool) []byte {
	entry := make([]byte, 18)
	for i := 0; i < 15; i++ {
		entry[i] = ' '
	}
	copy(entry[:15], []byte(name))
	entry[15] = suffix
	if group {
		binary.BigEndian.PutUint16(entry[16:18], 0x8000)
	}
	return entry
}
