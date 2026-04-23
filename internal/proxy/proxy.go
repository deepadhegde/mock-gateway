package proxy

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// Forward proxies r to targetBase+realPath, stripping all mock headers first.
func Forward(w http.ResponseWriter, r *http.Request, targetBase, realPath string) {
	r.Header.Del("X-Mock-Service")
	r.Header.Del("X-Mock-Enabled")
	r.Header.Del("X-Mock-Env")
	r.Header.Del("X-Mock-Token")

	target, err := url.Parse(targetBase)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy: bad target: %v", err), http.StatusBadGateway)
		return
	}

	// Propagate real client IP
	if clientIP, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if prior := r.Header.Get("X-Forwarded-For"); prior != "" {
			clientIP = prior + ", " + clientIP
		}
		r.Header.Set("X-Forwarded-For", clientIP)
	}
	if r.TLS != nil {
		r.Header.Set("X-Forwarded-Proto", "https")
	} else {
		r.Header.Set("X-Forwarded-Proto", "http")
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[proxy] upstream error %s %s → %s: %v", r.Method, r.URL.Path, targetBase, err)
		http.Error(w, fmt.Sprintf("proxy: upstream error: %v", err), http.StatusBadGateway)
	}

	r.URL.Scheme = target.Scheme
	r.URL.Host   = target.Host
	r.URL.Path   = realPath
	r.Host       = target.Host

	proxy.ServeHTTP(w, r)
}
