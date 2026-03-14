package router

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed panel.html
var panelHTMLTemplate string

// panelFieldSchema describes a single field in a procedure's input or output type.
type panelFieldSchema struct {
	Name     string                  `json:"name"`
	Type     string                  `json:"type"`
	JSONTag  string                  `json:"jsonTag"`
	Required bool                    `json:"required"`
	Fields   []panelFieldSchema `json:"fields,omitempty"`
	ElemType string                  `json:"elemType,omitempty"`
}

type panelProcInfo struct {
	Name         string                  `json:"name"`
	ShortName    string                  `json:"shortName"`
	Type         string                  `json:"type"`
	DefaultInput string                  `json:"defaultInput"`
	Schema       []panelFieldSchema `json:"schema"`
}

type panelGroup struct {
	Namespace  string
	Procedures []panelProcInfo
}

type panelTemplateData struct {
	BasePath       string
	Version        string
	Groups         []panelGroup
	ProceduresJSON template.JS
}

// servePanel serves the interactive panel UI.
// The handler is lazily initialized on the first request and cached.
// It also routes POST requests to __exec for htmx procedure execution.
func (r *Router) servePanel(w http.ResponseWriter, req *http.Request) {
	// Route __exec POST requests (check suffix to handle stripped prefixes)
	if req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/panel/__exec") {
		r.servePanelExec(w, req)
		return
	}

	r.panelOnce.Do(func() {
		r.panelHandler = r.buildPanelHandler()
	})
	r.panelHandler.ServeHTTP(w, req)
}

func (r *Router) buildPanelHandler() http.Handler {
	data := r.buildPanelData()
	tmpl := template.Must(template.New("playground").Parse(panelHTMLTemplate))

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.Execute(w, data)
	})
}

func (r *Router) buildPanelData() panelTemplateData {
	infos := r.ProcedureInfos()

	groupMap := make(map[string][]panelProcInfo)
	var allProcs []panelProcInfo

	for _, info := range infos {
		short := info.Name
		ns := ""
		if idx := strings.LastIndex(info.Name, "."); idx != -1 {
			ns = info.Name[:idx]
			short = info.Name[idx+1:]
		}

		pi := panelProcInfo{
			Name:         info.Name,
			ShortName:    short,
			Type:         string(info.Type),
			DefaultInput: pgDefaultJSON(info.InputType),
			Schema:       pgTypeSchema(info.InputType),
		}
		allProcs = append(allProcs, pi)
		groupMap[ns] = append(groupMap[ns], pi)
	}

	nsKeys := make([]string, 0, len(groupMap))
	for ns := range groupMap {
		nsKeys = append(nsKeys, ns)
	}
	sort.Strings(nsKeys)

	groups := make([]panelGroup, 0, len(nsKeys))
	for _, ns := range nsKeys {
		displayNS := ns
		if displayNS == "" {
			displayNS = "root"
		}
		groups = append(groups, panelGroup{
			Namespace:  displayNS,
			Procedures: groupMap[ns],
		})
	}

	procsJSON, _ := json.Marshal(allProcs)

	return panelTemplateData{
		BasePath:       r.basePath,
		Version:        Version,
		Groups:         groups,
		ProceduresJSON: template.JS(procsJSON),
	}
}

// ── Panel exec endpoint (htmx) ─────────────────────────────

// servePanelExec handles POST requests from the panel UI.
// It executes a procedure and returns syntax-highlighted HTML with OOB swaps
// for the status bar and history list.
func (r *Router) servePanelExec(w http.ResponseWriter, req *http.Request) {
	if err := req.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := req.FormValue("name")
	input := req.FormValue("input")
	hdrsJSON := req.FormValue("headers")

	proc, ok := r.procedures[name]
	if !ok {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<div class="response-view"><span class="j-null">procedure not found: %s</span></div>`, html.EscapeString(name))
		_, _ = fmt.Fprintf(w, `<span id="status-chip" class="status-chip s-err" hx-swap-oob="true">404</span>`)
		_, _ = fmt.Fprintf(w, `<span id="status-text" hx-swap-oob="true">procedure not found</span>`)
		_, _ = fmt.Fprintf(w, `<span id="status-time" hx-swap-oob="true" style="margin-left:auto"></span>`)
		return
	}

	if proc.Type == ProcedureSubscription {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<div class="response-view"><span class="j-null">use Connect for subscription procedures</span></div>`)
		_, _ = fmt.Fprintf(w, `<span id="status-chip" class="status-chip s-idle" hx-swap-oob="true">idle</span>`)
		_, _ = fmt.Fprintf(w, `<span id="status-text" hx-swap-oob="true">subscriptions use EventSource</span>`)
		_, _ = fmt.Fprintf(w, `<span id="status-time" hx-swap-oob="true" style="margin-left:auto"></span>`)
		return
	}

	var inputJSON []byte
	trimmed := strings.TrimSpace(input)
	if trimmed != "" && trimmed != "{}" {
		inputJSON = []byte(trimmed)
	}

	// Build synthetic request with custom headers
	method := http.MethodGet
	if proc.Type == ProcedureMutation {
		method = http.MethodPost
	}
	innerReq, _ := http.NewRequestWithContext(req.Context(), method, "/"+name, nil)
	if hdrsJSON != "" {
		var customHeaders map[string]string
		if err := json.Unmarshal([]byte(hdrsJSON), &customHeaders); err == nil {
			for k, v := range customHeaders {
				if k != "" {
					innerReq.Header.Set(k, v)
				}
			}
		}
	}

	rec := httptest.NewRecorder()
	start := time.Now()
	result := r.callProcedure(rec, innerReq, name, proc.Type, inputJSON)
	elapsed := time.Since(start).Milliseconds()

	// Determine HTTP status
	httpStatus := 200
	if result.Error != nil {
		httpStatus = result.Error.Data.HTTPStatus
	}

	// Format result as syntax-highlighted HTML
	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	highlighted := pgFormatJSON(string(jsonBytes))

	statusClass := "s-ok"
	statusText := fmt.Sprintf("%d OK", httpStatus)
	if httpStatus >= 400 {
		statusClass = "s-err"
		statusText = fmt.Sprintf("%d Error", httpStatus)
	}

	badgeCls := "b-q"
	badgeLetter := "Q"
	switch proc.Type {
	case ProcedureMutation:
		badgeCls = "b-m"
		badgeLetter = "M"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Main content → swapped into #response-body by htmx
	_, _ = fmt.Fprintf(w, `<div class="response-view">%s</div>`, highlighted)

	// OOB swaps for status bar
	_, _ = fmt.Fprintf(w, `<span id="status-chip" class="status-chip %s" hx-swap-oob="true">%d</span>`, statusClass, httpStatus)
	_, _ = fmt.Fprintf(w, `<span id="status-text" hx-swap-oob="true">%s</span>`, html.EscapeString(statusText))
	_, _ = fmt.Fprintf(w, `<span id="status-time" hx-swap-oob="true" style="margin-left:auto">%dms</span>`, elapsed)

	// OOB: prepend history row
	histStatusCls := "s-ok"
	if httpStatus >= 400 {
		histStatusCls = "s-err"
	}
	escapedName := html.EscapeString(name)
	jsName := template.JSEscapeString(name)
	_, _ = fmt.Fprintf(w, `<div id="history-list" hx-swap-oob="afterbegin"><div class="hist-row" onclick="selectProcByName('%s')"><span class="badge %s">%s</span><span class="hist-name">%s</span><span class="hist-time">%dms</span><span class="status-chip %s">%d</span></div></div>`,
		jsName, badgeCls, badgeLetter, escapedName, elapsed, histStatusCls, httpStatus)
}

// ── Server-side JSON syntax highlighting ────────────────────────

// pgFormatJSON parses a JSON string and returns syntax-highlighted HTML.
func pgFormatJSON(jsonStr string) string {
	var v interface{}
	if err := json.Unmarshal([]byte(jsonStr), &v); err != nil {
		return html.EscapeString(jsonStr)
	}
	var b strings.Builder
	pgWriteValue(&b, v, 0)
	return b.String()
}

func pgWriteValue(b *strings.Builder, v interface{}, depth int) {
	indent := strings.Repeat("  ", depth)
	switch val := v.(type) {
	case nil:
		b.WriteString(`<span class="j-null">null</span>`)
	case bool:
		if val {
			b.WriteString(`<span class="j-bool">true</span>`)
		} else {
			b.WriteString(`<span class="j-bool">false</span>`)
		}
	case float64:
		s := strconv.FormatFloat(val, 'f', -1, 64)
		fmt.Fprintf(b, `<span class="j-num">%s</span>`, s)
	case string:
		fmt.Fprintf(b, `<span class="j-str">"%s"</span>`, html.EscapeString(val))
	case []interface{}:
		if len(val) == 0 {
			b.WriteString(`<span class="j-brace">[]</span>`)
			return
		}
		b.WriteString("<span class=\"j-brace\">[</span>\n")
		for i, item := range val {
			b.WriteString(indent + "  ")
			pgWriteValue(b, item, depth+1)
			if i < len(val)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(indent)
		b.WriteString(`<span class="j-brace">]</span>`)
	case map[string]interface{}:
		if len(val) == 0 {
			b.WriteString(`<span class="j-brace">{}</span>`)
			return
		}
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("<span class=\"j-brace\">{</span>\n")
		for i, key := range keys {
			b.WriteString(indent + "  ")
			fmt.Fprintf(b, `<span class="j-key">"%s"</span>: `, html.EscapeString(key))
			pgWriteValue(b, val[key], depth+1)
			if i < len(keys)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(indent)
		b.WriteString(`<span class="j-brace">}</span>`)
	default:
		fmt.Fprintf(b, "%v", v)
	}
}

// ── Schema generation from reflect.Type ─────────────────────────

func pgTypeSchema(t reflect.Type) []panelFieldSchema {
	t = pgDeref(t)
	if t.Kind() != reflect.Struct {
		return nil
	}
	if t.NumField() == 0 {
		return []panelFieldSchema{}
	}

	fields := make([]panelFieldSchema, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts := pgParseJSONTag(tag)
		if name == "" {
			name = f.Name
		}
		required := !strings.Contains(opts, "omitempty")
		ft := pgDeref(f.Type)
		if f.Type.Kind() == reflect.Ptr {
			required = false
		}

		fs := panelFieldSchema{
			Name:     f.Name,
			Type:     pgGoTypeToSchema(ft),
			JSONTag:  name,
			Required: required,
		}
		if ft.Kind() == reflect.Struct && ft.NumField() > 0 && !pgIsTimeType(ft) {
			fs.Fields = pgTypeSchema(ft)
		}
		if ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array {
			elem := pgDeref(ft.Elem())
			fs.ElemType = pgGoTypeToSchema(elem)
			if elem.Kind() == reflect.Struct && elem.NumField() > 0 && !pgIsTimeType(elem) {
				fs.Fields = pgTypeSchema(elem)
			}
		}
		fields = append(fields, fs)
	}
	return fields
}

func pgDefaultJSON(t reflect.Type) string {
	t = pgDeref(t)
	if t.Kind() != reflect.Struct || t.NumField() == 0 {
		return "{}"
	}
	var b strings.Builder
	b.WriteString("{\n")
	first := true
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _ := pgParseJSONTag(tag)
		if name == "" {
			name = f.Name
		}
		if !first {
			b.WriteString(",\n")
		}
		first = false
		b.WriteString("  \"")
		b.WriteString(name)
		b.WriteString("\": ")
		b.WriteString(pgZeroValue(pgDeref(f.Type)))
	}
	b.WriteString("\n}")
	return b.String()
}

func pgZeroValue(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return `""`
	case reflect.Bool:
		return "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "0"
	case reflect.Slice, reflect.Array:
		return "[]"
	case reflect.Map, reflect.Struct:
		return "{}"
	default:
		return "null"
	}
}

func pgGoTypeToSchema(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	case reflect.Interface:
		return "any"
	default:
		return "any"
	}
}

func pgDeref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func pgParseJSONTag(tag string) (name, opts string) {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx], tag[idx+1:]
	}
	return tag, ""
}

func pgIsTimeType(t reflect.Type) bool {
	return t.PkgPath() == "time" && t.Name() == "Time"
}
