package main

import (
	"fmt"
	"net"
	"strings"
	"time"
)

func detect(host string) string {
	conn, err := net.DialTimeout("tcp", host, 2*time.Second)
	if err != nil {
		return "nicht erreichbar"
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(2 * time.Second))

	buf := make([]byte, 512)
	n, err := conn.Read(buf) // SSH sendet meist sofort ein Banner
	if err == nil && strings.HasPrefix(string(buf[:n]), "SSH-") {
		return "SSH"
	} else {
		fmt.Println(string(buf))
	}

	// HTTP aktiv anfragen
	fmt.Fprintf(conn, "HEAD / HTTP/1.0\r\nHost: %s\r\n\r\n",
		strings.Split(host, ":")[0])

	n, err = conn.Read(buf)
	if err == nil && strings.HasPrefix(string(buf[:n]), "HTTP/") {
		return "HTTP"
	} else {
		fmt.Println(string(buf))
	}

	return "unbekannt"
}

func main() {
	fmt.Println(detect("127.0.0.1:4096"))
	fmt.Println(detect("127.0.0.1:4097"))
	fmt.Println(detect("127.0.0.1:2222"))
	fmt.Println(detect("127.0.0.1:2223"))
}
