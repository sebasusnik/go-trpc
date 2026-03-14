package router

import (
	"fmt"
	"sort"
	"strings"
)

// ANSI color helpers for terminal output.
const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
	colorItalic  = "\033[3m"
	colorCyan    = "\033[36m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorMagenta = "\033[35m"
	colorBlue    = "\033[34m"
)

// PrintRoutes prints all registered procedures in a formatted, colorized table.
func (r *Router) PrintRoutes(basePath string, addr string) {
	type route struct {
		kind, name, method, path string
	}

	// Normalize addr to host:port form.
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}

	names := make([]string, 0, len(r.procedures))
	for name := range r.procedures {
		names = append(names, name)
	}
	sort.Strings(names)

	routes := make([]route, 0, len(names))
	maxName := 0
	for _, name := range names {
		proc := r.procedures[name]
		method := "GET"
		kind := "query"
		switch proc.Type {
		case ProcedureMutation:
			method = "POST"
			kind = "mutation"
		case ProcedureSubscription:
			kind = "subscription"
		}
		rt := route{kind: kind, name: name, method: method, path: basePath + "/" + name}
		if len(name) > maxName {
			maxName = len(name)
		}
		routes = append(routes, rt)
	}

	fmt.Println()
	fmt.Printf("  %s%sgo-trpc %s%s  %s%d procedures%s\n",
		colorBold, colorCyan, Version, colorReset,
		colorDim, len(routes), colorReset)
	fmt.Println()

	for _, rt := range routes {
		var kindColor string
		switch rt.kind {
		case "query":
			kindColor = colorGreen
		case "mutation":
			kindColor = colorYellow
		case "subscription":
			kindColor = colorMagenta
		}
		fmt.Printf("  %s%s%-14s%s %s%-*s%s  %s%-4s%s  %s%s%s\n",
			colorBold, kindColor, rt.kind, colorReset,
			colorBold, maxName, rt.name, colorReset,
			colorDim, rt.method, colorReset,
			colorDim, rt.path, colorReset)
	}

	if !r.disablePanel {
		fmt.Println()
		panelURL := "http://" + host + basePath + "/panel"
		// OSC 8 hyperlink: \033]8;;URL\033\\TEXT\033]8;;\033\\
		fmt.Printf("  %s%sPanel%s → \033]8;;%s\033\\%s%s%s\033]8;;\033\\%s\n",
			colorBold, colorCyan, colorReset,
			panelURL, colorCyan, colorBold, panelURL, colorReset)
	}
	fmt.Println()
}
