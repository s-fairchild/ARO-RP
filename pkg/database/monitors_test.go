package database

// Copyright (c) Microsoft Corporation.
// Licensed under the Apache License 2.0.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Azure/ARO-RP/pkg/database/cosmosdb"
	"github.com/sirupsen/logrus"
	"github.com/ugorji/go/codec"
)

func TestNewMonitors(t *testing.T) {
	ctx := context.Background()
	log := logrus.NewEntry(logrus.StandardLogger())

	for _, tt := range []struct {
		name    string
		wantErr string
		dbName  string
		status int
	}{
		{
			name: "pass: create new monitors",
			status: http.StatusCreated,
			dbName: "testdb",
		},
		{
			name: "fail: DATABASE_NAME is unset",
			wantErr: "environment variable \"DATABASE_NAME\" unset (development mode)",
		},
		{
			name: "fail: http status conflict",
			wantErr: "404 : ",
			dbName: "testdb",
			status: http.StatusNotFound,
		},
	} {
		if tt.dbName != "" {
			t.Setenv("DATABASE_NAME", tt.dbName)
		}

		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.String() == "/dbs/testdb/colls/Monitors/triggers" {
				w.WriteHeader(tt.status)
			} else {
				t.Logf("Resource not found: %s", r.URL.String())
			}
		}))
		
		host := strings.SplitAfter(ts.URL, "//")

		dbc := cosmosdb.NewDatabaseClient(log, ts.Client(), &codec.JsonHandle{}, host[1], nil)

		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMonitors(ctx, true, dbc)
			if err != nil && err.Error() != tt.wantErr ||
				err == nil && tt.wantErr != "" {
				t.Errorf("\n%v\n !=\n%v", err, tt.wantErr)
			}
		})

		os.Unsetenv("DATABASE_NAME")
	}
}
