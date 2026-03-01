package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"gopkg.in/yaml.v3"
)

// Formatter is the interface for output formatting.
type Formatter interface {
	Format(data any, writer io.Writer) error
}

// JSONFormatter outputs data as indented JSON.
type JSONFormatter struct{}

// Format writes data as JSON with 2-space indentation.
func (f *JSONFormatter) Format(data any, writer io.Writer) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON 직렬화 실패: %w", err)
	}
	b = append(b, '\n')
	_, err = writer.Write(b)
	return err
}

// YAMLFormatter outputs data as YAML.
type YAMLFormatter struct{}

// Format writes data as YAML.
func (f *YAMLFormatter) Format(data any, writer io.Writer) error {
	b, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("YAML 직렬화 실패: %w", err)
	}
	_, err = writer.Write(b)
	return err
}

// TableFormatter outputs data as an aligned table using tabwriter.
type TableFormatter struct {
	headers []string
	rowFunc func(any) []string
}

// NewTableFormatter creates a new TableFormatter with given headers and row extractor.
func NewTableFormatter(headers []string, rowFunc func(any) []string) *TableFormatter {
	return &TableFormatter{
		headers: headers,
		rowFunc: rowFunc,
	}
}

// Format writes data as an aligned table. data must be a slice of any.
func (f *TableFormatter) Format(data any, writer io.Writer) error {
	tw := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)

	// Write headers
	_, err := fmt.Fprintln(tw, strings.Join(f.headers, "\t"))
	if err != nil {
		return err
	}

	// Write rows from slice
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Slice {
		for i := 0; i < v.Len(); i++ {
			row := f.rowFunc(v.Index(i).Interface())
			_, err = fmt.Fprintln(tw, strings.Join(row, "\t"))
			if err != nil {
				return err
			}
		}
	}

	return tw.Flush()
}

// TextFormatter outputs data as "key: value" pairs or one-per-line for slices.
type TextFormatter struct{}

// Format writes data in text format depending on type.
func (f *TextFormatter) Format(data any, writer io.Writer) error {
	v := reflect.ValueOf(data)

	switch v.Kind() {
	case reflect.Map:
		return f.formatMap(v, writer)
	case reflect.Slice:
		return f.formatSlice(v, writer)
	default:
		_, err := fmt.Fprintf(writer, "%v\n", data)
		return err
	}
}

// formatMap writes map entries as sorted "key: value" lines.
func (f *TextFormatter) formatMap(v reflect.Value, writer io.Writer) error {
	keys := make([]string, 0, v.Len())
	for _, k := range v.MapKeys() {
		keys = append(keys, fmt.Sprintf("%v", k.Interface()))
	}
	sort.Strings(keys)

	for _, key := range keys {
		val := v.MapIndex(reflect.ValueOf(key))
		if _, err := fmt.Fprintf(writer, "%s: %v\n", key, val.Interface()); err != nil {
			return err
		}
	}
	return nil
}

// formatSlice writes each element on its own line.
func (f *TextFormatter) formatSlice(v reflect.Value, writer io.Writer) error {
	for i := 0; i < v.Len(); i++ {
		if _, err := fmt.Fprintf(writer, "%v\n", v.Index(i).Interface()); err != nil {
			return err
		}
	}
	return nil
}

// DetailFormatter outputs map data in a human-readable detail view.
// Top-level scalar fields are shown as "Label: value" with aligned padding.
// Map values are shown with indented key-value pairs.
// Slice values are shown as indented lists or mini-tables.
type DetailFormatter struct {
	// fieldOrder defines which top-level keys to display and in what order.
	// Keys not in fieldOrder are displayed after ordered keys alphabetically.
	fieldOrder []string
	// labelMap provides display labels for keys (e.g., "node_id" → "Node ID").
	labelMap map[string]string
	// sectionKeys lists keys that should be rendered as separate sections.
	sectionKeys map[string]bool
}

// NewDetailFormatter creates a new DetailFormatter.
func NewDetailFormatter(fieldOrder []string, labelMap map[string]string, sectionKeys map[string]bool) *DetailFormatter {
	return &DetailFormatter{
		fieldOrder:  fieldOrder,
		labelMap:    labelMap,
		sectionKeys: sectionKeys,
	}
}

// Format writes data in a structured detail format.
func (f *DetailFormatter) Format(data any, writer io.Writer) error {
	m, ok := data.(map[string]any)
	if !ok {
		_, err := fmt.Fprintf(writer, "%v\n", data)
		return err
	}

	// 표시할 키 순서 결정
	orderedKeys := f.orderedKeys(m)

	// 라벨 최대 길이 계산 (섹션 키 제외)
	maxLabelLen := 0
	for _, key := range orderedKeys {
		if f.sectionKeys[key] {
			continue
		}
		label := f.label(key)
		if len(label) > maxLabelLen {
			maxLabelLen = len(label)
		}
	}

	// 스칼라 필드 먼저 출력
	for _, key := range orderedKeys {
		if f.sectionKeys[key] {
			continue
		}
		val, exists := m[key]
		if !exists {
			continue
		}
		label := f.label(key)
		padding := strings.Repeat(" ", maxLabelLen-len(label))
		fmt.Fprintf(writer, "%s:%s  %v\n", label, padding, val)
	}

	// 섹션 필드 출력
	for _, key := range orderedKeys {
		if !f.sectionKeys[key] {
			continue
		}
		val, exists := m[key]
		if !exists {
			continue
		}
		label := f.label(key)
		fmt.Fprintf(writer, "\n%s:\n", label)
		f.formatSection(writer, val)
	}

	return nil
}

// label returns the display label for a key.
func (f *DetailFormatter) label(key string) string {
	if f.labelMap != nil {
		if label, ok := f.labelMap[key]; ok {
			return label
		}
	}
	return key
}

// orderedKeys returns keys in display order.
func (f *DetailFormatter) orderedKeys(m map[string]any) []string {
	seen := make(map[string]bool)
	var result []string

	// 지정된 순서의 키 먼저
	for _, key := range f.fieldOrder {
		if _, exists := m[key]; exists {
			result = append(result, key)
			seen[key] = true
		}
	}

	// 나머지 키 알파벳순
	var remaining []string
	for key := range m {
		if !seen[key] {
			remaining = append(remaining, key)
		}
	}
	sort.Strings(remaining)
	result = append(result, remaining...)

	return result
}

// formatSection renders a section value with indentation.
func (f *DetailFormatter) formatSection(writer io.Writer, val any) {
	switch v := val.(type) {
	case map[string]any:
		f.formatSectionMap(writer, v, "  ")
	case []any:
		f.formatSectionSlice(writer, v, "  ")
	default:
		fmt.Fprintf(writer, "  %v\n", val)
	}
}

// formatSectionMap renders a map with indentation.
func (f *DetailFormatter) formatSectionMap(writer io.Writer, m map[string]any, indent string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		val := m[key]
		switch v := val.(type) {
		case map[string]any:
			fmt.Fprintf(writer, "%s%s:\n", indent, key)
			f.formatSectionMap(writer, v, indent+"  ")
		case []any:
			fmt.Fprintf(writer, "%s%s:\n", indent, key)
			f.formatSectionSlice(writer, v, indent+"  ")
		default:
			_ = v
			fmt.Fprintf(writer, "%s%s: %v\n", indent, key, val)
		}
	}
}

// formatSectionSlice renders a slice with indentation.
// If all elements are maps with the same keys, renders as a mini-table.
// Otherwise renders as a list.
func (f *DetailFormatter) formatSectionSlice(writer io.Writer, items []any, indent string) {
	if len(items) == 0 {
		fmt.Fprintf(writer, "%s(empty)\n", indent)
		return
	}

	// 모든 요소가 맵인지 확인
	if f.isUniformMapSlice(items) {
		f.formatMiniTable(writer, items, indent)
		return
	}

	// 단순 리스트 (맵 항목은 재귀적으로 렌더링)
	for _, item := range items {
		switch v := item.(type) {
		case map[string]any:
			fmt.Fprintf(writer, "%s-\n", indent)
			f.formatSectionMap(writer, v, indent+"  ")
		default:
			fmt.Fprintf(writer, "%s- %v\n", indent, item)
		}
	}
}

// isUniformMapSlice checks if all elements are maps with the same keys.
func (f *DetailFormatter) isUniformMapSlice(items []any) bool {
	if len(items) == 0 {
		return false
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		return false
	}
	keys := make(map[string]bool)
	for k := range first {
		keys[k] = true
	}
	for _, item := range items[1:] {
		m, ok := item.(map[string]any)
		if !ok {
			return false
		}
		if len(m) != len(keys) {
			return false
		}
		for k := range m {
			if !keys[k] {
				return false
			}
		}
	}
	return true
}

// formatMiniTable renders a slice of uniform maps as a mini-table.
func (f *DetailFormatter) formatMiniTable(writer io.Writer, items []any, indent string) {
	first := items[0].(map[string]any)

	// 키 순서: 주요 식별자 → 타입/상태 → 수치 → 나머지 알파벳순
	priorityKeys := []string{
		"id", "node_id", "name", "node_name",
		"type", "node_type", "direction", "state",
		"processed", "errors", "connected",
		"messages", "throughput", "active_for",
	}
	seen := make(map[string]bool)
	var headers []string
	for _, k := range priorityKeys {
		if _, exists := first[k]; exists {
			headers = append(headers, k)
			seen[k] = true
		}
	}
	var rest []string
	for k := range first {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	headers = append(headers, rest...)

	// 각 컬럼의 최대 너비 계산
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(strings.ToUpper(h))
	}
	for _, item := range items {
		m := item.(map[string]any)
		for i, h := range headers {
			val := fmt.Sprintf("%v", m[h])
			if len(val) > widths[i] {
				widths[i] = len(val)
			}
		}
	}

	// 헤더 출력
	var headerParts []string
	for i, h := range headers {
		headerParts = append(headerParts, fmt.Sprintf("%-*s", widths[i], strings.ToUpper(h)))
	}
	fmt.Fprintf(writer, "%s%s\n", indent, strings.Join(headerParts, "  "))

	// 행 출력
	for _, item := range items {
		m := item.(map[string]any)
		var rowParts []string
		for i, h := range headers {
			val := fmt.Sprintf("%-*v", widths[i], m[h])
			rowParts = append(rowParts, val)
		}
		fmt.Fprintf(writer, "%s%s\n", indent, strings.Join(rowParts, "  "))
	}
}

// NewFormatter creates a Formatter for the given format string.
// Supported: "json", "yaml", "table", "text".
func NewFormatter(format string) (Formatter, error) {
	switch format {
	case "json":
		return &JSONFormatter{}, nil
	case "yaml":
		return &YAMLFormatter{}, nil
	case "table":
		// Return a table formatter with no headers/rowFunc; use NewTableFormatter for customization
		return &TableFormatter{}, nil
	case "text":
		return &TextFormatter{}, nil
	default:
		return nil, fmt.Errorf("지원하지 않는 출력 형식입니다: %s. 사용 가능: json, yaml, table, text", format)
	}
}

// PrintResult is a high-level helper that creates the right formatter and outputs data.
// For "table" format, tableHeaders and rowFunc must be provided.
// If tableHeaders is nil and format is "table", it falls back to "text" format
// so that single objects (maps) are displayed as key-value pairs.
func PrintResult(w io.Writer, format string, data any, tableHeaders []string, rowFunc func(any) []string) error {
	if format == "table" && tableHeaders != nil {
		f := NewTableFormatter(tableHeaders, rowFunc)
		return f.Format(data, w)
	}

	// table 포맷에 headers 가 없으면 text 로 폴백하여 단일 객체도 표시한다.
	if format == "table" && tableHeaders == nil {
		format = "text"
	}

	f, err := NewFormatter(format)
	if err != nil {
		return err
	}
	return f.Format(data, w)
}

// IsColorEnabled returns whether color output should be enabled.
// Returns false if noColor flag is true or if writer is not a terminal.
func IsColorEnabled(noColor bool, writer io.Writer) bool {
	if noColor {
		return false
	}

	// Check if writer is a *os.File and is a terminal
	if f, ok := writer.(*os.File); ok {
		stat, err := f.Stat()
		if err != nil {
			return false
		}
		return (stat.Mode() & os.ModeCharDevice) != 0
	}

	return false
}

// spinnerChars defines the animation frames for the spinner.
var spinnerChars = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// StartSpinner starts a simple character animation spinner.
// Returns a stop function that halts the spinner and waits for cleanup.
func StartSpinner(w io.Writer, msg string) func() {
	var once sync.Once
	var wg sync.WaitGroup
	done := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-done:
				// Clear the spinner line
				fmt.Fprintf(w, "\r%s\r", strings.Repeat(" ", len(msg)+4))
				return
			default:
				fmt.Fprintf(w, "\r%c %s", spinnerChars[i%len(spinnerChars)], msg)
				i++
				time.Sleep(80 * time.Millisecond)
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(done)
			wg.Wait()
		})
	}
}
