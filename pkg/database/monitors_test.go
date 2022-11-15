package database

// Copyright (c) Microsoft Corporation.
// Licensed under the Apache License 2.0.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Azure/ARO-RP/pkg/api"
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

func TestMonitorsCreate(t *testing.T) {
	ctx := context.Background()
	log := logrus.NewEntry(logrus.StandardLogger())

	t.Setenv("DATABASE_NAME", "testdb")

	for _, tt := range []struct {
		name    string
		wantErr string
		status int
		doc *api.MonitorDocument
	}{
		{
			name: "fail: id isn't lowercase",
			doc: &api.MonitorDocument{
				ID: "UPPER",
			},
			wantErr: "id \"UPPER\" is not lower case",
			status: http.StatusCreated,
		},
		{
			name: "fail: http status conflict",
			doc: &api.MonitorDocument{
				ID: "id-1",
			},
			status: http.StatusConflict,
			wantErr: "412 : ",
		},
	} {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.String() == "/dbs/testdb/colls/Monitors/triggers" {
				w.WriteHeader(http.StatusCreated)
			} else if r.URL.String() == "/dbs/testdb/colls/Monitors/docs" {
				w.WriteHeader(tt.status)	
			} else {
				t.Logf("Resource not found: %s", r.URL.String())
			}
		}))
		
		host := strings.SplitAfter(ts.URL, "//")

		dbc := cosmosdb.NewDatabaseClient(log, ts.Client(), &codec.JsonHandle{}, host[1], nil)
		c, err := NewMonitors(ctx, true, dbc)
		if err != nil {
			t.Fatalf("\n%s\n failed to create new monitors: %s\n", tt.name, err.Error())
		}

		t.Run(tt.name, func(t *testing.T) {
			_, err := c.Create(ctx, tt.doc)
			if err != nil && err.Error() != tt.wantErr ||
				err == nil && tt.wantErr != "" {
				t.Errorf("\n%v\n !=\n%v", err, tt.wantErr)
			}
		})
	}
}

func TestMonitorsPatchWithLease(t *testing.T) {
	ctx := context.Background()
	log := logrus.NewEntry(logrus.StandardLogger())

	t.Setenv("DATABASE_NAME", "testdb")

	for _, tt := range []struct {
		name    string
		wantErr string
		status int
		doc *api.MonitorDocument
		f func(*api.MonitorDocument) error
	}{
		{
			name: "fail: lost lease",
			doc: &api.MonitorDocument{
				ID: "id-1",
				LeaseOwner: "different-uuid",
			},
			f: func (doc *api.MonitorDocument) error {
				return nil	
			},
			wantErr: "lost lease",
		},
		{
			name: "fail: id isn't lowercase",
			doc: &api.MonitorDocument{
				ID: "UPPER",
			},
			wantErr: "id \"UPPER\" is not lower case",
		},
		{
			name: "pass: Patch with lease",
			f: func (doc *api.MonitorDocument) error {
				return nil	
			},
			doc: &api.MonitorDocument{
				ID: "master",
				ETag: "tag",
			},
		},
	} {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch u := r.URL.String(); u {
			case "/dbs/testdb/colls/Monitors/triggers":
				w.WriteHeader(http.StatusCreated)
			case "/dbs/testdb/colls/Monitors/docs":
				buf := &bytes.Buffer{}
				err := codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(&api.MonitorDocuments{
						MonitorDocuments: []*api.MonitorDocument{
							tt.doc,
						},
					},
				)
				if err != nil {
					t.Fatalf("\n%s\nfailed to encode document to request body\n%v\n", tt.name, err)
				}

				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("x-ms-version", "2018-12-31")

				w.Write(buf.Bytes())
			case "/dbs/testdb/colls/Monitors/docs/"+tt.doc.ID:
				// use c.uuid from TryLease request to return doc with c.uuid as LeaseOwner
				if r.Body != http.NoBody {
					var newDoc *api.MonitorDocument
					err := codec.NewDecoder(r.Body, &codec.JsonHandle{}).Decode(&newDoc)
					if err != nil {
						t.Fatalf("\n%s\nfailed to decode document from request body\n%v\n", tt.name, err)
					}
					tt.doc = newDoc
				}

				buf := &bytes.Buffer{}
				err := codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(tt.doc)
				if err != nil {
					t.Logf("\n%s\nfailed to encode document to request body\n%v\n", tt.name, err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("x-ms-version", "2018-12-31")
				w.Write(buf.Bytes())
			default:
				w.WriteHeader(http.StatusNotFound)
				t.Logf("Resource not found: %s", r.URL.String())
			}
		}))
		
		host := strings.SplitAfter(ts.URL, "//")

		dbc := cosmosdb.NewDatabaseClient(log, ts.Client(), &codec.JsonHandle{}, host[1], nil)
		c, err := NewMonitors(ctx, true, dbc)
		if err != nil {
			t.Fatalf("\n%s\n failed to create new monitors: %s\n", tt.name, err.Error())
		}

		// Use TryLease to get document with c's UUID as LeaseOwner
		if tt.doc.ID == "master" {
			tt.doc, err = c.TryLease(ctx)
			if err != nil {
				t.Fatalf("\n%s\n failed try lease: %s\n", tt.name, err.Error())
			}
		}

		t.Run(tt.name, func(t *testing.T) {
			_, err := c.PatchWithLease(ctx, tt.doc.ID, tt.f)
			if err != nil && err.Error() != tt.wantErr ||
				err == nil && tt.wantErr != "" {
				t.Errorf("\n%v\n !=\n%v", err, tt.wantErr)
			}
		})
	}
}
