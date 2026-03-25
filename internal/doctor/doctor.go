package doctor

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

type Check struct {
	Name    string `json:"name"`
	Status  Status `json:"status"`
	Message string `json:"message"`
}

type Report struct {
	Tool      ToolInfo `json:"tool"`
	Status    Status   `json:"status"`
	Checks    []Check  `json:"checks"`
	Timestamp string   `json:"timestamp"`
}

type ToolInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func Run(ctx context.Context, dsn string, version string) Report {
	r := Report{
		Tool:      ToolInfo{Name: "mongopulse", Version: version},
		Status:    StatusPass,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	// Check 1: connectivity.
	client, err := mongo.Connect(options.Client().ApplyURI(dsn).SetTimeout(10 * time.Second))
	if err != nil {
		r.Checks = append(r.Checks, Check{Name: "connectivity", Status: StatusFail, Message: fmt.Sprintf("connect: %v", err)})
		r.Status = StatusFail
		return r
	}
	defer client.Disconnect(ctx)

	if err := client.Ping(ctx, nil); err != nil {
		r.Checks = append(r.Checks, Check{Name: "connectivity", Status: StatusFail, Message: fmt.Sprintf("ping: %v", err)})
		r.Status = StatusFail
		return r
	}
	r.Checks = append(r.Checks, Check{Name: "connectivity", Status: StatusPass, Message: "connected"})

	// Check 2: server version.
	var status bson.M
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&status); err != nil {
		r.Checks = append(r.Checks, Check{Name: "server_version", Status: StatusFail, Message: fmt.Sprintf("serverStatus: %v", err)})
		r.downgrade(StatusFail)
		return r
	}

	if v, ok := status["version"].(string); ok {
		r.Checks = append(r.Checks, Check{Name: "server_version", Status: StatusPass, Message: v})
	}

	// Check 3: replSetGetStatus (optional — standalone won't have it).
	var rsStatus bson.M
	err = client.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetGetStatus", Value: 1}}).Decode(&rsStatus)
	if err != nil {
		r.Checks = append(r.Checks, Check{Name: "replication", Status: StatusWarn, Message: "not a replica set member (standalone)"})
		r.downgrade(StatusWarn)
	} else {
		r.Checks = append(r.Checks, Check{Name: "replication", Status: StatusPass, Message: "replica set active"})
	}

	// Check 4: profiling level.
	var profileResult bson.M
	err = client.Database("admin").RunCommand(ctx, bson.D{{Key: "profile", Value: -1}}).Decode(&profileResult)
	if err != nil {
		r.Checks = append(r.Checks, Check{Name: "profiling", Status: StatusWarn, Message: fmt.Sprintf("cannot read profile level: %v", err)})
		r.downgrade(StatusWarn)
	} else {
		level, _ := profileResult["was"].(int32)
		switch level {
		case 0:
			r.Checks = append(r.Checks, Check{Name: "profiling", Status: StatusWarn, Message: "profiling disabled (level 0) — slow query collector will be inactive"})
			r.downgrade(StatusWarn)
		case 1:
			r.Checks = append(r.Checks, Check{Name: "profiling", Status: StatusPass, Message: "profiling level 1 (slow queries only)"})
		case 2:
			r.Checks = append(r.Checks, Check{Name: "profiling", Status: StatusPass, Message: "profiling level 2 (all queries)"})
		}
	}

	// Check 5: permissions — can we read system collections?
	_, err = client.Database("admin").Collection("system.version").Find(ctx, bson.D{}, options.Find().SetLimit(1))
	if err != nil {
		r.Checks = append(r.Checks, Check{Name: "permissions", Status: StatusWarn, Message: fmt.Sprintf("limited access to admin collections: %v", err)})
		r.downgrade(StatusWarn)
	} else {
		r.Checks = append(r.Checks, Check{Name: "permissions", Status: StatusPass, Message: "admin collection access OK"})
	}

	return r
}

func (r *Report) downgrade(s Status) {
	if s == StatusFail {
		r.Status = StatusFail
	} else if s == StatusWarn && r.Status == StatusPass {
		r.Status = StatusWarn
	}
}
