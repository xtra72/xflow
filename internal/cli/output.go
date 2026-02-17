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
func PrintResult(w io.Writer, format string, data any, tableHeaders []string, rowFunc func(any) []string) error {
	if format == "table" && tableHeaders != nil {
		f := NewTableFormatter(tableHeaders, rowFunc)
		return f.Format(data, w)
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
