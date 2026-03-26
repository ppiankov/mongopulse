package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/ppiankov/mongopulse/internal/config"
	"github.com/ppiankov/mongopulse/internal/doctor"
	"github.com/ppiankov/mongopulse/internal/snapshot"
)

type Server struct {
	client  *mongo.Client
	cfg     config.Config
	version string
}

func NewServer(cfg config.Config, version string) (*Server, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(cfg.DSN[0]).SetTimeout(10 * time.Second))
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	return &Server{client: client, cfg: cfg, version: version}, nil
}

func (s *Server) Close(ctx context.Context) {
	s.client.Disconnect(ctx)
}

// JSON-RPC types.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []toolContent `json:"content"`
}

func (s *Server) Run(ctx context.Context) error {
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		resp := s.handle(ctx, req)
		if err := encoder.Encode(resp); err != nil {
			slog.Error("encode response failed", "error", err)
		}
	}
}

func (s *Server) handle(ctx context.Context, req request) response {
	switch req.Method {
	case "initialize":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]interface{}{"name": "mongopulse", "version": s.version},
		}}
	case "tools/list":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]interface{}{"tools": s.toolList()}}
	case "tools/call":
		return s.handleToolCall(ctx, req)
	case "notifications/initialized":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]interface{}{}}
	default:
		return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "method not found"}}
	}
}

func (s *Server) toolList() []toolDef {
	obj := map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	return []toolDef{
		{Name: "mongopulse_status", Description: "One-shot cluster health snapshot", InputSchema: obj},
		{Name: "mongopulse_doctor", Description: "Diagnose connectivity and permissions", InputSchema: obj},
		{Name: "mongopulse_ls", Description: "List databases or collections", InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{
				"database": map[string]string{"type": "string", "description": "Database name (omit for all databases)"},
			},
		}},
		{Name: "mongopulse_top", Description: "Show running operations", InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{
				"min_elapsed": map[string]string{"type": "number", "description": "Minimum seconds running"},
			},
		}},
		{Name: "mongopulse_slow", Description: "Slow query digest", InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{
				"lookback": map[string]string{"type": "string", "description": "Time window (e.g. 24h)"},
				"top":      map[string]string{"type": "number", "description": "Number of patterns"},
			},
		}},
		{Name: "mongopulse_explain", Description: "Collection intelligence summary", InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{
				"collection": map[string]string{"type": "string", "description": "db.collection name"},
			}, "required": []string{"collection"},
		}},
		{Name: "mongopulse_who", Description: "Which clients query a collection", InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{
				"collection": map[string]string{"type": "string", "description": "db.collection name"},
				"lookback":   map[string]string{"type": "string", "description": "Time window (e.g. 7d)"},
			}, "required": []string{"collection"},
		}},
	}
}

func (s *Server) handleToolCall(ctx context.Context, req request) response {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid params"}}
	}

	var result interface{}
	var err error

	switch params.Name {
	case "mongopulse_status":
		snap := snapshot.Take(ctx, s.client, s.cfg.DSN[0], s.cfg.SlowQueryThreshold.Seconds())
		result = snap
	case "mongopulse_doctor":
		report := doctor.Run(ctx, s.cfg.DSN[0], s.version)
		result = report
	case "mongopulse_ls":
		result, err = s.mcpLs(ctx, params.Arguments)
	case "mongopulse_top":
		result, err = s.mcpTop(ctx, params.Arguments)
	case "mongopulse_explain":
		result, err = s.mcpExplain(ctx, params.Arguments)
	case "mongopulse_who":
		result, err = s.mcpWho(ctx, params.Arguments)
	case "mongopulse_slow":
		result, err = s.mcpSlow(ctx, params.Arguments)
	default:
		return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "unknown tool: " + params.Name}}
	}

	if err != nil {
		return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32000, Message: err.Error()}}
	}

	b, _ := json.Marshal(result)
	return response{JSONRPC: "2.0", ID: req.ID, Result: toolResult{Content: []toolContent{{Type: "text", Text: string(b)}}}}
}

func (s *Server) mcpLs(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var p struct {
		Database string `json:"database"`
	}
	json.Unmarshal(args, &p)

	if p.Database != "" {
		return s.lsCollections(ctx, p.Database)
	}
	return s.lsDatabases(ctx)
}

func (s *Server) lsDatabases(ctx context.Context) (interface{}, error) {
	result, err := s.client.ListDatabases(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	type dbEntry struct {
		Name      string `json:"name"`
		SizeBytes int64  `json:"size_bytes"`
	}
	var dbs []dbEntry
	for _, db := range result.Databases {
		if db.Name == "admin" || db.Name == "local" || db.Name == "config" {
			continue
		}
		dbs = append(dbs, dbEntry{Name: db.Name, SizeBytes: db.SizeOnDisk})
	}
	return dbs, nil
}

func (s *Server) lsCollections(ctx context.Context, dbName string) (interface{}, error) {
	colls, err := s.client.Database(dbName).ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	return colls, nil
}

func (s *Server) mcpTop(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var p struct {
		MinElapsed float64 `json:"min_elapsed"`
	}
	json.Unmarshal(args, &p)

	var result bson.M
	err := s.client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "currentOp", Value: 1},
		{Key: "active", Value: true},
	}).Decode(&result)
	if err != nil {
		return nil, err
	}

	type opEntry struct {
		OpID   interface{} `json:"opid"`
		Op     string      `json:"op"`
		NS     string      `json:"ns"`
		Secs   float64     `json:"secs_running"`
		Client string      `json:"client"`
	}

	inprog, _ := result["inprog"].(bson.A)
	var ops []opEntry
	for _, raw := range inprog {
		op, ok := raw.(bson.M)
		if !ok {
			continue
		}
		secs := 0.0
		if v, ok := op["secs_running"].(int32); ok {
			secs = float64(v)
		}
		if secs < p.MinElapsed {
			continue
		}
		ns, _ := op["ns"].(string)
		client, _ := op["client"].(string)
		opType, _ := op["op"].(string)
		ops = append(ops, opEntry{OpID: op["opid"], Op: opType, NS: ns, Secs: secs, Client: client})
	}
	return ops, nil
}

func (s *Server) mcpExplain(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var p struct {
		Collection string `json:"collection"`
	}
	if err := json.Unmarshal(args, &p); err != nil || p.Collection == "" {
		return nil, fmt.Errorf("collection is required")
	}

	dbName, collName := splitNS(p.Collection)
	if collName == "" {
		return nil, fmt.Errorf("format: db.collection")
	}

	db := s.client.Database(dbName)
	var stats bson.M
	if err := db.RunCommand(ctx, bson.D{{Key: "collStats", Value: collName}}).Decode(&stats); err != nil {
		return nil, err
	}
	return stats, nil
}

func (s *Server) mcpWho(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var p struct {
		Collection string `json:"collection"`
		Lookback   string `json:"lookback"`
	}
	if err := json.Unmarshal(args, &p); err != nil || p.Collection == "" {
		return nil, fmt.Errorf("collection is required")
	}
	if p.Lookback == "" {
		p.Lookback = "7d"
	}

	dbName, collName := splitNS(p.Collection)
	if collName == "" {
		return nil, fmt.Errorf("format: db.collection")
	}

	lookback := 7 * 24 * time.Hour
	if len(p.Lookback) > 1 && p.Lookback[len(p.Lookback)-1] == 'd' {
		var days int
		fmt.Sscanf(p.Lookback, "%dd", &days)
		lookback = time.Duration(days) * 24 * time.Hour
	}

	cutoff := time.Now().Add(-lookback)
	targetNS := dbName + "." + collName

	coll := s.client.Database(dbName).Collection("system.profile")
	filter := bson.D{
		{Key: "ns", Value: targetNS},
		{Key: "ts", Value: bson.D{{Key: "$gte", Value: cutoff}}},
	}
	cursor, err := coll.Find(ctx, filter, options.Find().SetLimit(1000))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	type clientEntry struct {
		Client  string `json:"client"`
		AppName string `json:"app_name"`
		Count   int    `json:"count"`
	}

	clientMap := make(map[string]*clientEntry)
	for cursor.Next(ctx) {
		var doc bson.M
		cursor.Decode(&doc)
		client, _ := doc["client"].(string)
		appName, _ := doc["appName"].(string)
		key := client
		if appName != "" {
			key = appName
		}
		if key == "" {
			key = "(unknown)"
		}
		if _, ok := clientMap[key]; !ok {
			clientMap[key] = &clientEntry{Client: client, AppName: appName}
		}
		clientMap[key].Count++
	}

	var resultList []clientEntry
	for _, c := range clientMap {
		resultList = append(resultList, *c)
	}
	return resultList, nil
}

func (s *Server) mcpSlow(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var p struct {
		Lookback string `json:"lookback"`
		Top      int    `json:"top"`
	}
	json.Unmarshal(args, &p)
	if p.Top == 0 {
		p.Top = 20
	}

	// Simplified — return profiler entries directly.
	dbs, err := s.client.ListDatabaseNames(ctx, bson.D{})
	if err != nil {
		return nil, err
	}

	type slowEntry struct {
		DB         string  `json:"db"`
		NS         string  `json:"ns"`
		Op         string  `json:"op"`
		MillisSecs float64 `json:"millis"`
	}

	var entries []slowEntry
	for _, dbName := range dbs {
		if dbName == "admin" || dbName == "local" || dbName == "config" {
			continue
		}
		coll := s.client.Database(dbName).Collection("system.profile")
		cursor, err := coll.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "millis", Value: -1}}).SetLimit(int64(p.Top)))
		if err != nil {
			continue
		}
		for cursor.Next(ctx) {
			var doc bson.M
			cursor.Decode(&doc)
			ns, _ := doc["ns"].(string)
			op, _ := doc["op"].(string)
			millis := 0.0
			if v, ok := doc["millis"].(int32); ok {
				millis = float64(v)
			}
			entries = append(entries, slowEntry{DB: dbName, NS: ns, Op: op, MillisSecs: millis})
		}
		cursor.Close(ctx)
	}
	return entries, nil
}

func splitNS(ns string) (string, string) {
	for i := 0; i < len(ns); i++ {
		if ns[i] == '.' {
			return ns[:i], ns[i+1:]
		}
	}
	return ns, ""
}
