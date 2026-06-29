package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Context struct {
	Req    Request
	Params map[string]string
}

type APIError struct {
	StatusCode int
	Message    string
	Err        error
}

func (e *APIError) Error() string {
	return e.Message
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

type App struct {
	FileDirectory string
}

var statusText = map[int]string{
	200: "OK",
	201: "Created",
	404: "Not Found",
	400: "invalid input",
}

func NewAPIError(Status int, message string, cause error) *APIError {
	return &APIError{StatusCode: Status, Message: message, Err: cause}
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

type HandlerFunc func(ctx *Context) (Response, error)
type Middleware func(next HandlerFunc) HandlerFunc

func Chain(handler HandlerFunc, middlewares ...Middleware) HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	return handler
}

func NoOpMiddleware(next HandlerFunc) HandlerFunc {
	return func(ctx *Context) (Response, error) {
		return next(ctx)
	}
}

func LogMiddleware(next HandlerFunc) HandlerFunc {
	return func(ctx *Context) (Response, error) {
		start := time.Now()
		res, err := next(ctx)
		duration := time.Since(start)

		fmt.Printf("method=%s path=%s status=%d duration=%s\n",
			ctx.Req.Method, ctx.Req.Path, res.StatusCode, duration)
		return res, err
	}
}

func ErrorHandlingMiddleware(next HandlerFunc) HandlerFunc {
	return func(ctx *Context) (Response, error) {
		res, err := next(ctx)
		if err == nil {
			return res, nil
		}

		apiErr, ok := err.(*APIError)
		if !ok {
			apiErr = NewAPIError(500, "internal error", err)
		}

		fmt.Printf("level=ERROR method=%s path=%s status=%d message=%q cause=%v\n",
			ctx.Req.Method, ctx.Req.Path, apiErr.StatusCode, apiErr.Message, apiErr.Err)

		return EmptyResponse(apiErr.StatusCode), nil
	}
}

func RecoverMiddleware(next HandlerFunc) HandlerFunc {
	return func(ctx *Context) (res Response, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = NewAPIError(500, "internal error", fmt.Errorf("%v", r))
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

type Config struct {
	Host          string
	Port          int
	FileDirectory string
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

func LoadConfig() Config {
	host := flag.String("host", "0.0.0.0", "host to listen on")
	port := flag.Int("port", 4221, "port to listen")
	dir := flag.String("directory", "", "file directory")

	flag.Parse()

	return Config{
		Host:          *host,
		Port:          *port,
		FileDirectory: *dir,
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
	bufferSize = 1024
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

func handleEcho(ctx *Context) (Response, error) {
	return TextResponse(200, ctx.Param("str")), nil
}

func handleUserAgent(ctx *Context) (Response, error) {
	return TextResponse(200, ctx.Req.Headers["User-Agent"]), nil
}

func (a *App) handleFileGet(ctx *Context) (Response, error) {
	directory := a.FileDirectory
	content, err := os.ReadFile(directory + ctx.Param("filename"))
	if err != nil {
		return Response{}, NewAPIError(404, "File not found", nil)
	}

	return FileResponse(200, content), nil
}

func (a *App) handleFilePost(ctx *Context) (Response, error) {
	directory := a.FileDirectory
	fullPath := directory + ctx.Param("filename")

	if !handleWriteFile(fullPath, ctx.Req.Body) {
		return Response{}, NewAPIError(404, "File cannot created , Permission pervilage", nil)
	}

	return EmptyResponse(201), nil
}

func handleId(ctx *Context) (Response, error) {
	return TextResponse(200, "user id: "+ctx.Param("id")), nil
}

func handleRoot(ctx *Context) (Response, error) {
	return EmptyResponse(200), nil
}

func handlePanic(ctx *Context) (Response, error) {
	var m map[string]string
	m["this"] = "panics" // write to nil map — guaranteed panic
	return EmptyResponse(200), nil
}

func handleSlow(ctx *Context) (Response, error) {
	time.Sleep(10 * time.Second)
	return TextResponse(200, "slow response"), nil
}

func hundleConnection(conn net.Conn, wg *sync.WaitGroup) {
	defer wg.Done()
	defer conn.Close()

	request, err := ReadRequest(conn)
	if err != nil {
		fmt.Println("Error when handling request", err)
		return
	}

	req := requestParse(request)
	if handler, params, ok := router.Lookup(req.Method, req.Path); ok {
		ctx := &Context{Req: req, Params: params}
		res, _ := handler(ctx)
		conn.Write(res.Bytes())
		return
	}

	conn.Write(EmptyResponse(404).Bytes())
}

func registerAllRoutes(conf Config) {
	app := &App{FileDirectory: conf.FileDirectory}

	router.GET("/files/{filename}", Chain(app.handleFileGet, ErrorHandlingMiddleware, LogMiddleware, RecoverMiddleware))

	router.POST("/files/{filename}", Chain(app.handleFilePost, ErrorHandlingMiddleware, LogMiddleware, RecoverMiddleware))

	router.GET("/slow", Chain(handleSlow, ErrorHandlingMiddleware, LogMiddleware, RecoverMiddleware))
	router.GET("/", handleRoot)
	router.GET("/echo/{str}", Chain(handleEcho, ErrorHandlingMiddleware, LogMiddleware, RecoverMiddleware))
	router.GET("/user-agent", handleUserAgent)
	router.GET("/users/{id}", Chain(handleId, ErrorHandlingMiddleware, NoOpMiddleware))
	router.GET("/panic", Chain(handlePanic, ErrorHandlingMiddleware, LogMiddleware, RecoverMiddleware))
}

func main() {
	config := LoadConfig()
	if config.FileDirectory == "" {
		fmt.Println("usage: server --directory <path>")
		os.Exit(1)
	}

	registerAllRoutes(config)

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("failed to bind a port 4221")
		os.Exit(1)
	}

	var wg sync.WaitGroup

	// Listen for SIGINT (Ctrl+C) or SIGTERM (Kubernetes)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	shutdown := make(chan struct{}) // NEW — signals main to wait

	// Shutdown goroutine — waits for signal, then drains
	go func() {
		sig := <-quit
		fmt.Printf("signal=%s shutting down...\n", sig)

		// Stop accepting new connections
		listener.Close()

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			fmt.Println("shutdown clean")
			time.Sleep(100 * time.Millisecond) // let stdout flush
		case <-time.After(30 * time.Second):
			fmt.Println("shutdown timeout — forcing exit")
			time.Sleep(100 * time.Millisecond)
		}
		close(shutdown) // NEW — tell main we're done
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			// listener.Close() causes Accept() to return an error —
			// this is the expected signal that we're shutting down
			fmt.Println("listener closed, stopping accept loop")
			break // NEW — break instead of return, so we reach the wait below
		}
		wg.Add(1)
		go hundleConnection(conn, &wg)
	}

	<-shutdown // NEW — block here until shutdown goroutine finishes
}
