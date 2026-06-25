package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Context struct {
	Req    Request
	Params map[string]string
}

func (c *Context) Param(key string) string {
	return c.Params[key]
}

type Request struct {
	Method  string
	Path    string
	Version string
	Headers map[string]string
	Body    string
}

type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

var statusText = map[int]string{
	200: "OK",
	201: "Created",
	404: "Not Found",
}

type APIError struct {
	StatusCode int
	Message    string
	Err        error
}

func (res Response) Bytes() []byte {
	text, ok := statusText[res.StatusCode]
	if !ok {
		text = ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("HTTP/1.1 %d %s\r\n", res.StatusCode, text))

	if res.Headers == nil {
		res.Headers = map[string]string{}
	}
	if len(res.Body) > 0 {
		if _, exists := res.Headers["Content-Length"]; !exists {
			res.Headers["Content-Length"] = strconv.Itoa(len(res.Body))
		}
	}
	for key, value := range res.Headers {
		sb.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}
	sb.WriteString("\r\n")

	return append([]byte(sb.String()), res.Body...)
}

type HandlerFunc func(ctx *Context) Response
type Middleware func(next HandlerFunc) HandlerFunc

func Chain(handler HandlerFunc, middlewares ...Middleware) HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	return handler
}

func NoOpMiddleware(next HandlerFunc) HandlerFunc {
	return func(ctx *Context) Response {
		return next(ctx)
	}
}

func LogMiddleware(next HandlerFunc) HandlerFunc {
	return func(ctx *Context) Response {
		start := time.Now()
		res := next(ctx)
		duration := time.Since(start)

		fmt.Printf("method=%s path=%s status=%d duration=%s\n",
			ctx.Req.Method, ctx.Req.Path, res.StatusCode, duration)
		return res
	}
}

func RecoverMiddleware(next HandlerFunc) HandlerFunc {
	return func(ctx *Context) (res Response) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Println("recovered from panic:", r)
				res = EmptyResponse(500)
			}
		}()
		return next(ctx)
	}
}

type Route struct {
	Method  string
	Pattern string
	Handler HandlerFunc
}

type Router struct {
	routes []Route
}

func TextResponse(status int, body string) Response {
	return Response{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "text/plain"},
		Body:       []byte(body),
	}
}

func FileResponse(status int, body []byte) Response {
	return Response{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/octet-stream"},
		Body:       body,
	}
}

func EmptyResponse(status int) Response {
	return Response{StatusCode: status}
}

func NewRouter() *Router {
	return &Router{
		routes: []Route{},
	}
}

func (r *Router) Handle(method, pattern string, handler HandlerFunc) {
	r.routes = append(r.routes, Route{Method: method, Pattern: pattern, Handler: handler})
}

func (r *Router) GET(pattern string, handler HandlerFunc) {
	r.Handle("GET", pattern, handler)
}

func (r *Router) POST(pattern string, handler HandlerFunc) {
	r.Handle("POST", pattern, handler)
}

func matchPath(pattern, path string) (map[string]string, bool) {
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")

	if len(patternParts) != len(pathParts) {
		return nil, false
	}

	params := make(map[string]string)
	for i, part := range patternParts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			key := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
			params[key] = pathParts[i]
		} else if part != pathParts[i] {
			return nil, false
		}
	}
	return params, true
}

func (r *Router) Lookup(method, path string) (HandlerFunc, map[string]string, bool) {
	for _, route := range r.routes {
		if route.Method != method {
			continue
		}

		if params, ok := matchPath(route.Pattern, path); ok {
			return route.Handler, params, true
		}
	}

	return nil, nil, false
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
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}

	return buf[:n], nil
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

func handleWriteFile(path string, content string) bool {
	err := os.WriteFile(path, []byte(content), 0644)
	return err == nil
}

var router = NewRouter()

func handleEcho(ctx *Context) Response {
	return TextResponse(200, ctx.Param("str"))
}

func handleUserAgent(ctx *Context) Response {
	return TextResponse(200, ctx.Req.Headers["User-Agent"])
}

func handleFileGet(ctx *Context) Response {
	if len(os.Args) < 3 {
		return EmptyResponse(404)
	}

	directory := os.Args[2]
	content, err := os.ReadFile(directory + ctx.Param("filename"))
	if err != nil {
		return EmptyResponse(404)
	}

	return FileResponse(200, content)
}

func handleFilePost(ctx *Context) Response {
	if len(os.Args) < 3 {
		return EmptyResponse(404)
	}
	directory := os.Args[2]
	fullPath := directory + ctx.Param("filename")

	if !handleWriteFile(fullPath, ctx.Req.Body) {
		return EmptyResponse(404)
	}

	return EmptyResponse(201)
}

func handleId(ctx *Context) Response {
	return TextResponse(200, "user id: "+ctx.Param("id"))
}

func handleRoot(ctx *Context) Response {
	return EmptyResponse(200)
}

func handlePanic(ctx *Context) Response {
	var m map[string]string
	m["this"] = "panics" // write to nil map — guaranteed panic
	return EmptyResponse(200)
}

func init() {
	router.GET("/", handleRoot)
	router.GET("/echo/{str}", Chain(handleEcho, LogMiddleware, RecoverMiddleware))
	router.GET("/user-agent", handleUserAgent)
	router.GET("/files/{filename}", handleFileGet)
	router.POST("/files/{filename}", handleFilePost)
	router.GET("/users/{id}", Chain(handleId, NoOpMiddleware))

	//test Panic
	router.GET("/panic", Chain(handlePanic, LogMiddleware, RecoverMiddleware))
}

func hundleConnection(conn net.Conn) {
	defer conn.Close()

	request, err := ReadRequest(conn)
	if err != nil {
		fmt.Println("Error when handling request", err)
		return
	}

	req := requestParse(request)

	if handler, params, ok := router.Lookup(req.Method, req.Path); ok {
		ctx := &Context{Req: req, Params: params}
		conn.Write(handler(ctx).Bytes())
		return
	}

	conn.Write(EmptyResponse(404).Bytes())
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
