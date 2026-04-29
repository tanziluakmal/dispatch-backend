package handlers

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type proxyRequestBody struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body,omitempty"`
}

type proxyHeaderPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type proxyResponseBody struct {
	Status     int               `json:"status"`
	StatusText string            `json:"statusText"`
	Headers    []proxyHeaderPair `json:"headers"`
	Body       string            `json:"body"`
	ElapsedMs  int64             `json:"elapsedMs"`
}

// ProxyRequest relays an HTTP request server-side for the Request Builder (browser CORS bypass).
func (s *Server) ProxyRequest(c *gin.Context) {
	_, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	var body proxyRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	urlStr := strings.TrimSpace(body.URL)
	if urlStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url required"})
		return
	}
	lower := strings.ToLower(urlStr)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url must start with http:// or https://"})
		return
	}

	method := strings.ToUpper(strings.TrimSpace(body.Method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodHead, http.MethodOptions:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported method"})
		return
	}

	var reqBody io.Reader
	if body.Body != "" && method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		reqBody = strings.NewReader(body.Body)
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), method, urlStr, reqBody)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for k, v := range body.Headers {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	started := time.Now()

	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	const maxBody = 10 << 20 // 10 MiB
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if len(raw) > maxBody {
		c.JSON(http.StatusBadGateway, gin.H{"error": "response body too large"})
		return
	}

	outHeaders := make([]proxyHeaderPair, 0)
	for name, vals := range resp.Header {
		for _, v := range vals {
			outHeaders = append(outHeaders, proxyHeaderPair{Name: name, Value: v})
		}
	}

	c.JSON(http.StatusOK, proxyResponseBody{
		Status:     resp.StatusCode,
		StatusText: resp.Status,
		Headers:    outHeaders,
		Body:       string(raw),
		ElapsedMs:  time.Since(started).Milliseconds(),
	})
}
