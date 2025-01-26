package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/buildbuddy-io/buildbuddy/server/util/log"

	dbpb "github.com/buildbuddy-io/buildbuddy/enterprise/tools/databuddy/proto/databuddy"
)

type ClickHouseClient struct {
	Hostname string
	Port     string
	Database string
}

func (ch *ClickHouseClient) Query(ctx context.Context, query string) (*dbpb.QueryResult, error) {
	cmd := exec.CommandContext(ctx,
		"clickhouse-client",
		"--host", ch.Hostname, "--port", ch.Port,
		"--database", ch.Database,
		"--query", query,
		"--format=JSONColumnsWithMetadata",
	)
	stdout := &bytes.Buffer{}
	cmd.Stdout = stdout
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if s := stderr.String(); s != "" {
			return &dbpb.QueryResult{
				Error: s,
			}, nil
		}
		return nil, fmt.Errorf("clickhouse-client: %w: %q", err, stderr)
	}
	if stdout.Len() == 0 {
		return nil, fmt.Errorf("clickhouse-client: empty output (missing query?)")
	}

	output := &JSONColumnsWithMetadata{}
	if err := json.Unmarshal(stdout.Bytes(), output); err != nil {
		return nil, fmt.Errorf("unmarshal stdout: %w (full stdout: %q)", err, stdout.String())
	}

	result := &dbpb.QueryResult{}
	for _, c := range output.Meta {
		clientType := clientColumnType(c.Type)
		result.Columns = append(result.Columns, &dbpb.Column{
			Name: c.Name,
			Data: toColumnData(clientType, output.Data[c.Name]),
		})
	}

	return result, nil
}

func toColumnData(clientType string, data []any) *dbpb.ColumnData {
	columnData := &dbpb.ColumnData{}

	if len(data) == 0 {
		return columnData
	}
	switch data[0].(type) {
	case float64:
		// TODO: avoid lossy float32 conversion
		columnData.FloatValues = make([]float32, 0, len(data))
		for _, v := range data {
			columnData.FloatValues = append(columnData.FloatValues, float32(v.(float64)))
		}
	case string:
		columnData.StringValues = make([]string, 0, len(data))
		for _, v := range data {
			columnData.StringValues = append(columnData.StringValues, v.(string))
		}
	default:
		log.Warningf("Unhandled column data type %T (data=%v, clientType=%s)", data[0], data, clientType)
	}

	return columnData
}

// Convert ClickHouse column type to client column type, e.g. Uint64 => Long
func clientColumnType(typ string) string {
	typ = strings.ToLower(typ)
	if strings.Contains(typ, "float") {
		return "Number"
	}
	if strings.Contains(typ, "64") {
		return "Long"
	}
	if strings.Contains(typ, "int") {
		return "Number"
	}
	return "String"
}

type JSONColumnsWithMetadata struct {
	Meta                   []ColumnMetadata `json:"meta"`
	Data                   map[string][]any `json:"data"`
	Rows                   int              `json:"rows"`
	RowsBeforeLimitAtLeast int              `json:"rows_before_limit_at_least,omitempty"`
}

type ColumnMetadata struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
