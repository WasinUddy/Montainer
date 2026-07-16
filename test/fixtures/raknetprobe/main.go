// Command raknetprobe sends one RakNet unconnected ping and prints the
// Bedrock server advertisement. It is intentionally dependency-free so the
// release gate can run it inside the candidate image's Docker network.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"time"
)

var offlineMagic = []byte{
	0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe,
	0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78,
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: raknetprobe HOST:PORT")
		os.Exit(2)
	}
	advertisement, err := ping(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(advertisement)
}

func ping(address string) (string, error) {
	remote, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return "", fmt.Errorf("resolve address: %w", err)
	}
	connection, err := net.DialUDP("udp4", nil, remote)
	if err != nil {
		return "", fmt.Errorf("dial address: %w", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return "", err
	}

	packet := make([]byte, 33)
	packet[0] = 0x01
	binary.BigEndian.PutUint64(packet[1:9], uint64(time.Now().UnixMilli()))
	copy(packet[9:25], offlineMagic)
	binary.BigEndian.PutUint64(packet[25:33], uint64(time.Now().UnixNano()))
	if _, err := connection.Write(packet); err != nil {
		return "", fmt.Errorf("send unconnected ping: %w", err)
	}

	response := make([]byte, 4096)
	count, err := connection.Read(response)
	if err != nil {
		return "", fmt.Errorf("read unconnected pong: %w", err)
	}
	response = response[:count]
	const payloadOffset = 33
	if len(response) < payloadOffset+2 || response[0] != 0x1c {
		return "", fmt.Errorf("unexpected pong packet of %d bytes", len(response))
	}
	if !bytes.Equal(response[17:33], offlineMagic) {
		return "", fmt.Errorf("pong has invalid offline magic")
	}
	length := int(binary.BigEndian.Uint16(response[payloadOffset : payloadOffset+2]))
	if length <= 0 || payloadOffset+2+length > len(response) {
		return "", fmt.Errorf("pong has invalid payload length %d", length)
	}
	return string(response[payloadOffset+2 : payloadOffset+2+length]), nil
}
