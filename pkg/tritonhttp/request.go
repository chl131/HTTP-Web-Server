package tritonhttp

import (
	"bufio"
	"fmt"
	"strings"
)

type Request struct {
	Method string // e.g. "GET"
	URL    string // e.g. "/path/to/a/file"
	Proto  string // e.g. "HTTP/1.1"

	// Header stores misc headers excluding "Host" and "Connection",
	// which are stored in special fields below.
	// Header keys are case-incensitive, and should be stored
	// in the canonical format in this map.
	Header map[string]string

	Host  string // determine from the "Host" header
	Close bool   // determine from the "Connection" header
}

// ReadRequest tries to read the next valid request from br.
//
// If it succeeds, it returns the valid request read. In this case,
// bytesReceived should be true, and err should be nil.
//
// If an error occurs during the reading, it returns the error,
// and a nil request. In this case, bytesReceived indicates whether or not
// some bytes are received before the error occurs. This is useful to determine
// the timeout with partial request received condition.
func ReadRequest(br *bufio.Reader) (req *Request, bytesReceived bool, err error) {
	req = &Request{}

	_, errByteReceived := br.Peek(1)
	if errByteReceived != nil {
		bytesReceived = false
		return nil, bytesReceived, errByteReceived
	} else {
		bytesReceived = true
	}

	// Read start line
	var requestLineArr []string

	for temp_line, err := ReadLine(br); temp_line != ""; {
		if err != nil {
			return nil, bytesReceived, err
		}
		requestLineArr = append(requestLineArr, temp_line)
		temp_line, err = ReadLine(br)
	}
	// fmt.Print(len(requestLineArr))

	// Deal with empty bad request
	if len(requestLineArr) == 0 {
		requestLineArr = append(requestLineArr, "Empty bad request")
	}

	// First element of a vaild arr should looks like "GET /images/myimg.jpg HTTP/1.1"
	startLine, err := parseStartLine(requestLineArr[0])
	if err != nil {
		return nil, bytesReceived, err
	}
	req.Method = startLine[0]
	req.URL = startLine[1]
	req.Proto = startLine[2]
	req.Close = false
	headerMap := make(map[string]string)

	if !validMethod(req.Method) {
		return nil, bytesReceived, fmt.Errorf("Bad Method.")
	}
	if !validURL(req.URL) {
		return nil, bytesReceived, fmt.Errorf("URL doesn't start with slash.")
	}
	if !validProto(req.Proto) {
		return nil, bytesReceived, fmt.Errorf("Bad Proto.")
	}

	// Read headers
	// Check required headers
	// Handle special headers
	haveHost := 0
	for _, header := range requestLineArr[1:] {
		key, val, err := parseHeader(header)

		if err != nil {
			return nil, bytesReceived, err
		}

		if key == "Host" {
			haveHost = 1
			req.Host = val
		} else if key == "Connection" {
			if val == "close" {
				req.Close = true // Deal with invalid connection instructions?
			}
		} else {
			headerMap[key] = val
		}
	}
	req.Header = headerMap
	// fmt.Print(req.Header)

	if haveHost == 0 {
		return nil, bytesReceived, fmt.Errorf("Missing Host.")
	}

	// Return valid request
	return req, bytesReceived, nil
}

// helper functions below

func parseStartLine(line string) ([]string, error) {
	fields := strings.SplitN(line, " ", 3)
	if len(fields) != 3 {
		return fields, fmt.Errorf("Bad start line: %v", fields)
	}
	return fields, nil
}

func parseHeader(header string) (string, string, error) {
	headerArr := strings.SplitN(header, ":", 2)
	if len(headerArr) != 2 {
		return "", "", fmt.Errorf("Bad header")
	}
	if !validKey(headerArr[0]) {
		return "", "", fmt.Errorf("Bad key")
	}

	key := CanonicalHeaderKey(headerArr[0])
	val := convertValue(headerArr[1])
	return key, val, nil
}

func validKey(key string) bool {
	if len(key) == 0 {
		return false
	}
	for _, r := range key {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && (r != '-') {
			return false
		}
	}
	return true
}

func convertValue(val string) string {
	val = strings.TrimLeft(val, " ")
	val = strings.TrimRight(val, "\r\n")
	return val
}

func validMethod(method string) bool {
	return method == "GET"
}

func validProto(proto string) bool {
	return proto == responseProto
}

func validURL(URL string) bool {
	if string(URL[0]) != "/" {
		return false
	}
	return true
}
