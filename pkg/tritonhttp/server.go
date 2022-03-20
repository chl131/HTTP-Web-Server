package tritonhttp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	responseProto          = "HTTP/1.1"
	statusOK               = 200
	statusMethodNotAllowed = 400
	fileNotFound           = 404
)

type Server struct {
	// Addr specifies the TCP address for the server to listen on,
	// in the form "host:port". It shall be passed to net.Listen()
	// during ListenAndServe().
	Addr string // e.g. ":0"

	// DocRoot specifies the path to the directory to serve static files from.
	DocRoot string
}

// ListenAndServe listens on the TCP network address s.Addr and then
// handles requests on incoming connections.
func (s *Server) ListenAndServe() error {
	// Validate the configuration of the server
	if err := s.ValidateServerSetup(); err != nil {
		return fmt.Errorf("server is not setup correctly %v", err)
	}

	// server should now start to listen on the configured address
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	fmt.Println("Listening on", ln.Addr())

	// making sure the listener is closed when we exit
	defer func() {
		err = ln.Close()
		if err != nil {
			fmt.Println("error in closing listener", err)
		}
	}()

	// accept connections forever
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		fmt.Println("accepted connection", conn.RemoteAddr())
		go s.HandleConnection(conn)
	}
}

func (s *Server) ValidateServerSetup() error {
	// Validating the doc root of the server
	fi, err := os.Stat(s.DocRoot)

	if os.IsNotExist(err) {
		return err
	}

	if !fi.IsDir() {
		return fmt.Errorf("doc root %q is not a directory", s.DocRoot)
	}

	return nil
}

// HandleConnection reads requests from the accepted conn and handles them.
func (s *Server) HandleConnection(conn net.Conn) {
	br := bufio.NewReader(conn)

	// Hint: use the other methods below

	for {
		// Set timeout
		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			log.Printf("Failed to set timeout for connection %v", conn)
			_ = conn.Close()
			return
		}

		// Try to read next request
		req, bytesReceived, err_req := ReadRequest(br)

		// Handle EOF
		if errors.Is(err_req, io.EOF) {
			log.Printf("Connection closed by %v", conn.RemoteAddr())
			_ = conn.Close()
			return
		}

		// Handle timeout
		errTimeOut, ok := err_req.(net.Error)
		if errTimeOut != nil {
			if ok && errTimeOut.Timeout() && !bytesReceived {
				log.Printf("Connection to %v timed out", conn.RemoteAddr())
				_ = conn.Close()
				return
			} else if ok && errTimeOut.Timeout() && bytesReceived {
				log.Printf("Connection to %v timed out, bad request", conn.RemoteAddr())
				res := &Response{}
				res.HandleBadRequestTimeOut()
				_ = res.Write(conn)
				_ = conn.Close()
				return
			}
		}

		// Handle bad request
		if err_req != nil {
			log.Printf("Handle bad request for error: %v", err_req)
			res := &Response{}
			res.HandleBadRequest()
			_ = res.Write(conn)
			_ = conn.Close()
			return
		}

		// Handle good request
		res := s.HandleGoodRequest(req)
		if err := res.Write(conn); err != nil {
			fmt.Println(err)
		}

		// Close conn if requested
		if req.Close {
			_ = conn.Close()
			return
		}
	}
}

// HandleGoodRequest handles the valid req and generates the corresponding res.
// filepath.Clean("/subdir1/../../../subdir2/index.html") -> \subdir2\index.html
func (s *Server) HandleGoodRequest(req *Request) (res *Response) {
	res = &Response{}

	// Deal with escape and check exist
	req_url := req.URL
	if string(req_url[len(req_url)-1]) == "/" {
		req_url += "index.html"
	}

	url_split_concat := s.DocRoot + req_url
	path := filepath.Clean(url_split_concat)
	url_split := strings.Split(path, "/")
	root_split := strings.Split(s.DocRoot, "/")
	for i, v := range root_split {
		if url_split[i] != v {
			res.HandleNotFound(req)
			return res
		}
	}

	// path := filepath.Clean(filepath.Join(s.DocRoot, req_url))

	if _, err := filepath.Rel(s.DocRoot, path); err != nil {
		res.HandleNotFound(req)
	} else {
		pathExist, _ := exists(path)
		if pathExist {
			fmt.Printf("%s\n", req_url)
			fmt.Printf("%s\n", path)
			res.HandleOK(req, path)
		} else {
			res.HandleNotFound(req)
		}
	}

	// Hint: use the other methods below

	return res
}

// HandleOK prepares res to be a 200 OK response
// ready to be written back to client.
func (res *Response) HandleOK(req *Request, path string) {
	res.StatusCode = statusOK
	res.Proto = responseProto
	res.FilePath = path
	res.Request = req
	resMap := make(map[string]string)

	file, err := os.Stat(path)
	if err != nil {
		fmt.Println(err)
	}

	ext := "." + strings.SplitN(path, ".", 2)[1]
	resMap["Content-Type"] = MIMETypeByExtension(ext)
	resMap["Date"] = FormatTime(time.Now())
	resMap["Content-Length"] = fmt.Sprint(file.Size())
	resMap["Last-Modified"] = fmt.Sprint(file.ModTime())
	if req.Close {
		resMap["Connection"] = "close"
	}

	res.Header = resMap

}

// HandleNotFound prepares res to be a 404 Not Found response
// ready to be written back to client.
func (res *Response) HandleNotFound(req *Request) {
	res.StatusCode = fileNotFound
	res.Proto = responseProto
	res.FilePath = ""
	res.Request = req
	resMap := make(map[string]string)

	resMap["Date"] = FormatTime(time.Now())
	if req.Close {
		resMap["Connection"] = "close"
	}

	res.Header = resMap

}

// HandleBadRequest prepares res to be a 400 Bad Request response
// ready to be written back to client.
func (res *Response) HandleBadRequest() {
	res.StatusCode = statusMethodNotAllowed
	res.Proto = responseProto
	res.FilePath = ""
	resMap := make(map[string]string)

	resMap["Connection"] = "close"
	resMap["Date"] = FormatTime(time.Now())

	res.Header = resMap
}

func (res *Response) HandleBadRequestTimeOut() {
	res.StatusCode = statusMethodNotAllowed
	res.Proto = responseProto
	res.FilePath = ""
	resMap := make(map[string]string)

	resMap["Connection"] = "close"
	resMap["Date"] = FormatTime(time.Now())

	res.Header = resMap
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	} else {
		return false, nil
	}
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}
