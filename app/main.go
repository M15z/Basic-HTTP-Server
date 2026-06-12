package main

import (
	"fmt"
	"net"
	"os"
	"strings"
)

const (
	host       = "0.0.0.0:4221"
	bufferSize = 1024
)

var (
	response200 = []byte("HTTP/1.1 200 OK\r\n\r\n")
	response404 = []byte("HTTP/1.1 404 Not Found\r\n\r\n")
)

func ReadRequest(conn net.Conn) ([]byte, error) {
	buf := make([]byte, bufferSize)
	_, err := conn.Read(buf)
	return buf, err
}

func isRootPath(req []byte) bool {
	// HTTP request line looks like: "GET / HTTP/1.1"
	// req[4] == '/' and req[5] == ' ' means the path is exactly "/"
	return len(req) > 5 && req[4] == '/' && req[5] == ' '
}

func extractPath(req []byte) string {
	reqString := string(req)
	lastLine := strings.Index(reqString, "\r\n")
	if lastLine == -1 {
		lastLine = strings.Index(reqString, "\n")
	}

	if lastLine == -1 {
		return ""
	}

	line := reqString[:lastLine]
	// "GET /echo/strawberry HTTP/1.1"
	parts := strings.Fields(line)

	if len(parts) < 2 {
		return ""
	}

	return parts[1]
}

func isEcho(req []byte) (string, bool) {
	path := extractPath(req)
	if strings.HasPrefix(path, "/echo/") {
		return strings.TrimPrefix(path, "/echo/"), true
	}
	return "", false
}

func buildEchoResponse(body string) []byte {
	response := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	return []byte(response)
}

func hundleConnection(conn net.Conn) {
	defer conn.Close()

	req, err := ReadRequest(conn)
	if err != nil {
		fmt.Println("Error when hendle request", err)
		os.Exit(1)
	}

	fmt.Println("RAW PATH:", extractPath(req)) // debug line

	if str, ok := isEcho(req); ok {
		conn.Write(buildEchoResponse(str))
	} else if isRootPath(req) {
		conn.Write(response200)
	} else {
		conn.Write(response404)
	}
}

func main() {
	listener, err := net.Listen("tcp", host)
	if err != nil {
		fmt.Println("failed to bind a port 4221")
		os.Exit(1)
	}

	for i := 0; i < 2; i++ {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error Accpeting the message")
			os.Exit(1)
		}
		hundleConnection(conn)
	}
}
