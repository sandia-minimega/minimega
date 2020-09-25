package web

import "strings"

type ServerOption func(*serverOptions)

type serverOptions struct {
	endpoint  string
	jwtKey    string
	users     []string
	allowCORS bool

	logMiddleware string

	publishLogs  bool
	phenixLogs   string
	minimegaLogs string
}

func newServerOptions(opts ...ServerOption) serverOptions {
	o := serverOptions{
		endpoint:  ":3000",
		users:     []string{"admin@foo.com:foobar:Global Admin"},
		allowCORS: true, // TODO: default to false
	}

	for _, opt := range opts {
		opt(&o)
	}

	if o.phenixLogs != "" || o.minimegaLogs != "" {
		o.publishLogs = true
	}

	return o
}

func ServeOnEndpoint(e string) ServerOption {
	return func(o *serverOptions) {
		o.endpoint = e
	}
}

func ServeWithJWTKey(k string) ServerOption {
	return func(o *serverOptions) {
		o.jwtKey = k
	}
}

func ServeWithUsers(u string) ServerOption {
	return func(o *serverOptions) {
		o.users = strings.Split(u, "|")
	}
}

func ServeWithCORS(c bool) ServerOption {
	return func(o *serverOptions) {
		o.allowCORS = c
	}
}

func ServeWithMiddlewareLogging(l string) ServerOption {
	return func(o *serverOptions) {
		o.logMiddleware = l
	}
}

func ServePhenixLogs(p string) ServerOption {
	return func(o *serverOptions) {
		o.phenixLogs = p
	}
}

func ServeMinimegaLogs(m string) ServerOption {
	return func(o *serverOptions) {
		o.minimegaLogs = m
	}
}
