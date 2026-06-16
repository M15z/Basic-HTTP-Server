package main

import (
	"fmt"
	"net"
	"os"
	"strings"
)

type Request struct {
	Method  string
	Path    string
	Version string
	Headers map[string]string
	Body    string
}

const (
	host       = "0.0.0.0:4221"
	bufferSize = 1024
)

var (
	response200 = []byte("HTTP/1.1 200 OK\r\n\r\n")
	response404 = []byte("HTTP/1.1 404 Not Found\r\n\r\n")
	response201 = []byte("HTTP/1.1 201 Created\r\n\r\n")
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

func requestParse(req []byte) Request {
	reqString := strings.TrimRight(string(req), "\x00")
	section := strings.Split(reqString, "\r\n")

	// Parse request line: "GET /path HTTP/1.1"
	requestLine := strings.Fields(section[0])
	r := Request{
		Method:  requestLine[0],
		Path:    requestLine[1],
		Version: requestLine[2],
		Headers: make(map[string]string),
	}

	// Parse Header
	i := 1
	for i < len(section) && section[i] != "" {
		parts := strings.SplitN(section[i], ": ", 2)
		if len(parts) == 2 {
			r.Headers[parts[0]] = parts[1]
		}

		i++
	}

	// Everything after the empty line is the body
	if i+1 < len(section) {
		r.Body = strings.Join(section[i+1:], "\r\n")
	}

	return r

}

func isEcho(req []byte) (string, bool) {
	path := requestParse(req).Path
	if strings.HasPrefix(path, "/echo/") {
		return strings.TrimPrefix(path, "/echo/"), true
	}
	return "", false
}

func isUserAgent(req []byte) (string, bool) {
	r := requestParse(req)
	userAgent, ok := r.Headers["User-Agent"]
	return userAgent, ok
}

func handleWriteFile(path string, content string) (string, bool) {
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		return "", false
	}
	return "WRITE_OK", true
}

func isFile(req []byte) (string, bool) {
	r := requestParse(req)
	directory := os.Args[2] // assuming you pass --directory flag like the challenge expects

	if !strings.HasPrefix(r.Path, "/files/") {
		return "", false
	}

	filename := strings.TrimPrefix(r.Path, "/files/")
	fullPath := directory + filename

	if r.Method == "GET" {
		return filename, true
	}

	if r.Method == "POST" {
		return handleWriteFile(fullPath, r.Body)
	}

	return "", false
}

func buildFileResponse(filename string) []byte {
	directory := os.Args[2] // assuming you pass --directory flag like the challenge expects
	contents, err := os.ReadFile(directory + filename)
	if err != nil {
		return response404
	}
	response := fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n%s",
		len(contents),
		contents,
	)
	return []byte(response)
}

func buildResponse(body string) []byte {
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

	if str, ok := isEcho(req); ok {
		conn.Write(buildResponse(str))
	} else if str, ok := isUserAgent(req); ok {
		conn.Write(buildResponse(str))
	} else if result, ok := isFile(req); ok {
		if result == "WRITE_OK" {
			conn.Write(response201)
		} else {
			conn.Write(buildFileResponse(result))
		}
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

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error Accpeting the message")
			os.Exit(1)
		}
		go hundleConnection(conn)
	}
}
