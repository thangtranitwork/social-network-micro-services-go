package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"social-network-go/logger"

	"github.com/gin-gonic/gin"
)

// ProxyTo returns a gin.HandlerFunc that reverse-proxies requests to the target URL
func ProxyTo(target string) gin.HandlerFunc {
	targetUrl, err := url.Parse(target)
	if err != nil {
		logger.Err(err).Fatal("Invalid proxy target URL")
	}
	proxy := httputil.NewSingleHostReverseProxy(targetUrl)

	// Strip CORS headers from upstream responses — Gateway is the sole CORS authority
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Access-Control-Allow-Origin")
		resp.Header.Del("Access-Control-Allow-Methods")
		resp.Header.Del("Access-Control-Allow-Headers")
		resp.Header.Del("Access-Control-Allow-Credentials")
		resp.Header.Del("Access-Control-Expose-Headers")
		resp.Header.Del("Vary")
		return nil
	}

	return func(c *gin.Context) {
		logger.Info("[PROXY] %s %s -> %s%s", c.Request.Method, c.Request.URL.Path, target, c.Request.URL.Path)
		proxy.ServeHTTP(c.Writer, c.Request)
	}
}
