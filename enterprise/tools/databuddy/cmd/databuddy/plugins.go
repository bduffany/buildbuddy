package main

import (
	"context"

	dbpb "github.com/buildbuddy-io/buildbuddy/enterprise/tools/databuddy/proto/databuddy"
)

type ResultsProcessor interface {
	ProcessResults(ctx context.Context, res *dbpb.QueryResult) error
}

type Plugin interface{}

type groupSlugPlugin struct {
}

func NewGroupSlugPlugin(datasources []string) Plugin {
	return nil
}
