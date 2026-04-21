package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// Forward proxies r to targetBase+realPath, stripping all mock headers first.
func Forward(w http.ResponseWriter, r *http.Request, targetBase, realPath string) {
	// Strip mock headers — service must never see them
	r.Header.Del("X-Mock-Service")
	r.Header.Del("X-Mock-Enabled")
	r.Header.Del("X-Mock-Env")
	r.Header.Del("X-Mock-Token")

	target, err := url.Parse(targetBase)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy: bad target: %v", err), http.StatusBadGateway)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, fmt.Sprintf("proxy: upstream error: %v", err), http.StatusBadGateway)
	}

	// Rewrite the request URL to point at the real service
	r.URL.Scheme = target.Scheme
	r.URL.Host   = target.Host
	r.URL.Path   = realPath
	r.Host       = target.Host

	proxy.ServeHTTP(w, r)
}
