package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"gtihub.com/mwildt/git-works/pkg/auth"
)

var sharedTransport = &http.Transport{
	MaxIdleConns:          200,
	MaxIdleConnsPerHost:   100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ResponseHeaderTimeout: 30 * time.Second,
}

type proxyEntry struct {
	proxy        *httputil.ReverseProxy
	mu           sync.Mutex
	ref          auth.WorkloadReference
	lastActivity time.Time
	timer        *time.Timer
}

func (e *proxyEntry) touch(ref auth.WorkloadReference, idleAfter time.Duration, onTimeout func()) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ref = ref
	e.lastActivity = time.Now()
	if e.timer == nil {
		e.timer = time.AfterFunc(idleAfter, onTimeout)
	} else {
		e.timer.Reset(idleAfter)
	}
}

type proxyRegistry struct {
	mu        sync.Mutex
	entries   map[string]*proxyEntry
	idleAfter time.Duration
	onIdle    func(ctx context.Context, ref auth.WorkloadReference, lastActivity time.Time)
}

func newProxyRegistry(idleAfter time.Duration, onIdle func(ctx context.Context, ref auth.WorkloadReference, lastActivity time.Time)) *proxyRegistry {
	return &proxyRegistry{
		entries:   map[string]*proxyEntry{},
		idleAfter: idleAfter,
		onIdle:    onIdle,
	}
}

func (r *proxyRegistry) For(target *url.URL, ref auth.WorkloadReference) *httputil.ReverseProxy {
	key := target.String()
	r.mu.Lock()
	entry, ok := r.entries[key]
	if !ok {
		entry = &proxyEntry{proxy: newWorkloadProxy(target)}
		r.entries[key] = entry
	}
	r.mu.Unlock()
	entry.touch(ref, r.idleAfter, func() { r.expire(key, entry) })
	return entry.proxy
}

func (r *proxyRegistry) expire(key string, entry *proxyEntry) {
	entry.mu.Lock()
	idle := time.Since(entry.lastActivity) >= r.idleAfter
	ref := entry.ref
	lastActivity := entry.lastActivity
	if idle {
		entry.timer = nil
	} else if entry.timer != nil {
		entry.timer.Reset(r.idleAfter)
	}
	entry.mu.Unlock()
	if idle {
		r.onIdle(context.Background(), ref, lastActivity)
	}
}

func newWorkloadProxy(target *url.URL) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = sharedTransport
	proxy.FlushInterval = -1
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Default().Error("proxy error", "method", r.Method, "path", r.URL.Path, "error", err)
		http.Error(w, "workload unreachable", http.StatusBadGateway)
	}
	proxy.ModifyResponse = injectMixin
	return proxy
}

func injectMixin(resp *http.Response) error {
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		return fmt.Errorf("read upstream body: %w", err)
	}
	resp.Body.Close()

	const injectTag = "<script type=\"module\" crossorigin src=\"/.gw-mixin/index.js\" defer></script>"
	lower := bytes.ToLower(body)
	idx := bytes.LastIndex(lower, []byte("</body>"))
	if idx == -1 {
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		resp.Header.Set("Content-Length", fmt.Sprint(len(body)))
		return nil
	}
	var out bytes.Buffer
	out.Write(body[:idx])
	out.WriteString(injectTag)
	out.Write(body[idx:])

	resp.Body = io.NopCloser(&out)
	resp.ContentLength = int64(out.Len())
	resp.Header.Set("Content-Length", fmt.Sprint(out.Len()))
	return nil
}
