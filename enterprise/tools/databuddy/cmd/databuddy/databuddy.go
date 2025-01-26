package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/buildbuddy-io/buildbuddy/enterprise/server/backends/configsecrets"
	"github.com/buildbuddy-io/buildbuddy/enterprise/tools/databuddy/bundle"
	"github.com/buildbuddy-io/buildbuddy/server/backends/blobstore"
	"github.com/buildbuddy-io/buildbuddy/server/config"
	"github.com/buildbuddy-io/buildbuddy/server/interfaces"
	"github.com/buildbuddy-io/buildbuddy/server/util/db"
	"github.com/buildbuddy-io/buildbuddy/server/util/flag"
	"github.com/buildbuddy-io/buildbuddy/server/util/flagutil"
	"github.com/buildbuddy-io/buildbuddy/server/util/healthcheck"
	"github.com/buildbuddy-io/buildbuddy/server/util/log"
	"google.golang.org/protobuf/encoding/protojson"

	dbpb "github.com/buildbuddy-io/buildbuddy/enterprise/tools/databuddy/proto/databuddy"

	_ "embed"
)

var (
	listenAddr = flag.String("databuddy.http.listen", ":8083", "TCP address to listen on")
	// TODO: multiple data sources, with some id for each (so we can choose
	// between datasources in the UI).
	dataSource = flag.String("databuddy.data_sources", "clickhouse://localhost:9000/buildbuddy_local", "Data source")
)

//go:embed static/index.html
var indexHTML []byte

//go:embed static/styles.css
var stylesCSS []byte

func main() {
	flag.Parse()
	if err := run(); err != nil {
		log.Fatal(err.Error())
	}
}

func run() error {
	if err := configsecrets.Configure(); err != nil {
		return fmt.Errorf("configure config secrets: %w", err)
	}
	if err := config.Load(); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	ctx := context.Background()

	if err := log.Configure(); err != nil {
		return fmt.Errorf("configure logging: %w", err)
	}

	ds, err := NewDataSource(ctx, *dataSource)
	if err != nil {
		return fmt.Errorf("init data source %q: %w", *dataSource, err)
	}
	log.Infof("Configured data source %q", stripURLCredentials(*dataSource))

	oltpDSN, err := flagutil.GetDereferencedValue[string]("database.data_source")
	if err != nil {
		return fmt.Errorf("lookup 'database.data_source' config: %w", err)
	}
	gormDB, _, err := db.Open(ctx, oltpDSN, &db.AdvancedConfig{})
	if err != nil {
		return fmt.Errorf("open OLTP db: %w", err)
	}

	log.Infof("Starting OLTP database migrations")
	if err := gormDB.AutoMigrate(&Query{}); err != nil {
		return fmt.Errorf("migrate DB: %w", err)
	}
	log.Infof("Completed OLTP database migrations")

	queryDB := NewQueryDB(gormDB)

	log.Infof("Configured OLTP DB %q", stripURLCredentials(oltpDSN))

	blobstore, err := blobstore.NewFromConfig(ctx)
	if err != nil {
		return fmt.Errorf("init blobstore from configuration: %w", err)
	}

	server := &Server{
		db:         queryDB,
		blobstore:  blobstore,
		datasource: ds,
	}

	hc := healthcheck.NewHealthChecker("databuddy")

	mux := http.NewServeMux()

	mux.Handle("/styles.css", SetContentType(ServeContentWithETagCaching(bytes.NewReader(stylesCSS)), "text/css"))
	mux.Handle("/index.js", SetContentType(ServeContentWithETagCaching(bytes.NewReader(bundle.IndexJS)), "application/javascript"))
	mux.Handle("/monaco/vs-theme.css", SetContentType(ServeContentWithETagCaching(bytes.NewReader(bundle.MonacoCSS)), "text/css"))
	mux.Handle("/monaco/editor.worker.js", SetContentType(ServeContentWithETagCaching(bytes.NewReader(bundle.MonacoWorkerJS)), "application/javascript"))
	mux.Handle("/uplot/uplot.min.css", SetContentType(ServeContentWithETagCaching(bytes.NewReader(bundle.UPlotCSS)), "text/css"))

	mux.Handle("/", WithAuth(http.HandlerFunc(server.ServeIndex)))
	mux.Handle("/api/queries", APIHandler(server.GetQueries))
	mux.Handle("/api/query/", APIHandler(server.GetQuery))
	mux.Handle("/api/save/", APIHandler(server.SaveQuery))
	mux.Handle("/api/execute/", APIHandler(server.ExecuteQuery))
	mux.Handle("/api/schema", APIHandler(server.GetSchema))

	mux.Handle("/healthz", hc.LivenessHandler())
	mux.Handle("/readyz", hc.ReadinessHandler())

	// TODO: pprof, metrics

	lis, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		return fmt.Errorf("listen on %q: %w", *listenAddr, err)
	}
	hc.RegisterShutdownFunction(func(ctx context.Context) error {
		return lis.Close()
	})

	log.Printf("Listening on %s", *listenAddr)
	if err := http.Serve(lis, LogServerErrors(mux)); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

type Server struct {
	db        QueryDB
	blobstore interfaces.Blobstore

	datasource *DataSource
}

func (s *Server) ServeIndex(w http.ResponseWriter, r *http.Request) {
	html := string(indexHTML)
	initialDataJSON, err := protojson.Marshal(&dbpb.InitialData{
		User: AuthenticatedUserEmail(r.Context()),
	})
	if err != nil {
		log.Errorf("Failed to marshal initial data: %s", err)
		initialDataJSON = []byte("{}")
	}
	html = strings.ReplaceAll(html, `<script id="initial-data"></script>`, `<script>window._initialData = `+string(initialDataJSON)+`;</script>`)
	io.WriteString(w, html)
}

func (s *Server) GetQueries(r *http.Request) (*dbpb.QueriesResponse, error) {
	rows, err := s.db.GetQueries(r.Context())
	if err != nil {
		return nil, err
	}

	queries := make([]*dbpb.QueryMetadata, 0, len(rows))
	for _, row := range rows {
		queries = append(queries, QueryToProto(&row))
	}

	return &dbpb.QueriesResponse{Queries: queries}, nil
}

func (s *Server) GetQuery(r *http.Request) (*dbpb.GetQueryResponse, error) {
	queryID := strings.TrimPrefix(r.URL.Path, "/api/query/")
	if queryID == "" {
		return nil, BadRequest(fmt.Errorf("missing query ID in path"))
	}
	if strings.Contains(queryID, "..") {
		return nil, BadRequest(fmt.Errorf("query ID cannot contain '..'"))
	}
	q, sql, err := LookupQuery(r.Context(), s.db, s.blobstore, queryID)
	if err != nil {
		return nil, err
	}

	md := QueryToProto(q)

	var charts []*dbpb.Chart
	var parseError string
	config, err := ParseYAMLComments(sql)
	if err != nil {
		parseError = err.Error()
	} else {
		c, err := ParseCharts(config)
		if err != nil {
			parseError = fmt.Errorf("parse charts section: %w", err).Error()
		}
		charts = c
	}

	// TODO: do this in parallel with DB lookup
	cachedResult, err := GetCachedQueryResult(r.Context(), s.blobstore, queryID)
	if err != nil && !IsNotFound(err) {
		log.Printf("Failed to read cached query %q: %s", queryID, err)
	}

	res := &dbpb.GetQueryResponse{
		Metadata:     md,
		Sql:          sql,
		CachedResult: cachedResult,
		Charts:       charts,
		ParseError:   parseError,
	}
	return res, nil
}

func (s *Server) SaveQuery(r *http.Request) (*dbpb.SaveQueryResponse, error) {
	queryID := strings.TrimPrefix(r.URL.Path, "/api/save/")
	if queryID == "" {
		return nil, BadRequest(fmt.Errorf("missing query ID in path"))
	}
	if strings.Contains(queryID, "..") {
		return nil, BadRequest(fmt.Errorf("query ID cannot contain '..'"))
	}
	sql := r.URL.Query().Get("query")
	if err := SaveQuery(r.Context(), s.db, s.blobstore, queryID, sql); err != nil {
		return nil, err
	}

	var charts []*dbpb.Chart
	var parseError string
	config, err := ParseYAMLComments(sql)
	if err != nil {
		parseError = err.Error()
	} else {
		// TODO: let client parse the charts?
		c, err := ParseCharts(config)
		if err != nil {
			parseError = fmt.Errorf("parse charts section: %w", err).Error()
		} else {
			charts = c
		}
	}

	return &dbpb.SaveQueryResponse{
		Charts:     charts,
		ParseError: parseError,
	}, nil
}

func (s *Server) ExecuteQuery(r *http.Request) (*dbpb.ExecuteQueryResponse, error) {
	queryID := strings.TrimPrefix(r.URL.Path, "/api/execute/")
	if queryID == "" {
		return nil, BadRequest(fmt.Errorf("missing query ID in path"))
	}
	if strings.Contains(queryID, "..") {
		return nil, BadRequest(fmt.Errorf("query ID cannot contain '..'"))
	}
	query := r.URL.Query().Get("query")
	if err := SaveQuery(r.Context(), s.db, s.blobstore, queryID, query); err != nil {
		return nil, err
	}

	query, err := ResolveMacros(r.Context(), s.db, s.blobstore, query)
	if err != nil {
		return nil, err
	}

	// TODO: rely on users/permissions to prevent mutates, not parsing.
	if err := ValidateSQL(query); err != nil {
		return nil, BadRequest(err)
	}

	var charts []*dbpb.Chart
	var parseError string
	config, err := ParseYAMLComments(query)
	if err != nil {
		parseError = err.Error()
	} else {
		// TODO: let client parse the charts?
		c, err := ParseCharts(config)
		if err != nil {
			parseError = fmt.Errorf("parse charts section: %w", err).Error()
		} else {
			charts = c
		}
	}

	if config != nil && config.Macro {
		return nil, BadRequest(fmt.Errorf("macro definitions cannot be executed"))
	}

	start := time.Now()
	log.Infof("Running query %q", query)
	res, err := s.datasource.Query(r.Context(), query)
	if err != nil {
		return nil, err
	}
	log.Infof("Query completed in %s", time.Since(start))

	if err := CacheQueryResult(r.Context(), s.blobstore, queryID, res); err != nil {
		log.Printf("Failed to cache result: %s", err)
	}

	return &dbpb.ExecuteQueryResponse{
		Result:     res,
		Charts:     charts,
		ParseError: parseError,
	}, nil
}

func (s *Server) GetSchema(r *http.Request) (*dbpb.GetSchemaResponse, error) {
	return &dbpb.GetSchemaResponse{}, nil
}
