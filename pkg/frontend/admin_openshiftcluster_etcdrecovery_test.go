package frontend

// Copyright (c) Microsoft Corporation.
// Licensed under the Apache License 2.0.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	operatorv1client "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1"
	operatorv1fake "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1/fake"
	"github.com/sirupsen/logrus"
	kschema "k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/Azure/ARO-RP/pkg/api"
	"github.com/Azure/ARO-RP/pkg/env"
	"github.com/Azure/ARO-RP/pkg/frontend/adminactions"
	"github.com/Azure/ARO-RP/pkg/metrics/noop"
	mock_adminactions "github.com/Azure/ARO-RP/pkg/util/mocks/adminactions"
	mock_frontend "github.com/Azure/ARO-RP/pkg/util/mocks/frontend"
)

func TestAdminEtcdRecovery(t *testing.T) {
	mockSubID := "00000000-0000-0000-0000-000000000000"
	mockTenantID := mockSubID
	method := http.MethodPost
	resourceName := "cluster"
	ctx := context.Background()
	type test struct {
		name                    string
		mocks                   func(ctx context.Context, ti *testInfra, fix *mock_frontend.MockFixEtcd, k *mock_adminactions.MockKubeActions, log *logrus.Entry, env env.Interface, doc *api.OpenShiftClusterDocument, etcdcli operatorv1client.EtcdInterface)
		wantStatusCode          int
		wantResponse            []byte
		wantResponseContentType string
		wantError               string
		headers                 http.Header
	}

	for _, tt := range []*test{
		{
			name:                    "fail: misspelled header content type",
			wantStatusCode: http.StatusOK,
			wantResponseContentType: "application/json",
			// 			wantResponse: []byte(`{
			//     "error": {
			// 	    "code": "UnsupportedMediaType",
			// 	    "message": "The content media type 'applicaion/json' is not supported. Only 'application/json' is supported."
			//    }
			// }`),
			mocks: func(ctx context.Context, ti *testInfra, fix *mock_frontend.MockFixEtcd, k *mock_adminactions.MockKubeActions, log *logrus.Entry, env env.Interface, doc *api.OpenShiftClusterDocument, etcdcli operatorv1client.EtcdInterface) {
				k.EXPECT().ResolveGVR("Etcd").AnyTimes().Return(&kschema.GroupVersionResource{
					Group: "",
					Version: "v1",
					Resource: "Etcd",
				}, nil)
				// fix.EXPECT().Fix(ctx, ti.log, env, doc, k, etcdcli).MinTimes(1).Return([]byte("Pod 1 logs... \n Pod 2 logs..."), errors.New("Error happened"))
			},
			headers: http.Header{
				"Content-Type": []string{"application/json"},
			},
		},
	} {
		t.Run(fmt.Sprintf("%s: %s", method, tt.name), func(t *testing.T) {
			resourceID := fmt.Sprintf("/subscriptions/%s/resourcegroups/resourceGroup/providers/Microsoft.RedHatOpenShift/openShiftClusters/%s", mockSubID, resourceName)
						kubeConfig := api.SecureBytes(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://server
name: cluster
`)
			doc := &api.OpenShiftClusterDocument{
				Key: strings.ToLower(resourceID),
				OpenShiftCluster: &api.OpenShiftCluster{
					ID:   resourceID,
					Name: resourceName,
					Type: "Microsoft.RedHatOpenShift/openshiftClusters",
					Properties: api.OpenShiftClusterProperties{
						NetworkProfile: api.NetworkProfile{
							APIServerPrivateEndpointIP: "0.0.0.0",
						},
						AdminKubeconfig: kubeConfig,
						KubeadminPassword: api.SecureString("p"),
					},
				},
			}

			ti := newTestInfra(t).WithOpenShiftClusters().WithSubscriptions()
			defer ti.done()

			ti.fixture.AddOpenShiftClusterDocuments(doc)
			ti.fixture.AddSubscriptionDocuments(&api.SubscriptionDocument{
				ID: mockSubID,
				Subscription: &api.Subscription{
					State: api.SubscriptionStateRegistered,
					Properties: &api.SubscriptionProperties{
						TenantID: mockTenantID,
					},
				},
			})

			err := ti.buildFixtures(nil)
			if err != nil {
				t.Fatal(err)
			}

			k := mock_adminactions.NewMockKubeActions(ti.controller)
			if tt.mocks != nil {
				fix := mock_frontend.NewMockFixEtcd(ti.controller)
				tt.mocks(ctx, ti, fix, k, ti.log, ti.env, doc, &operatorv1fake.FakeEtcds{})
			}

			f, err := NewFrontend(ctx, ti.audit, ti.log, ti.env, ti.asyncOperationsDatabase, ti.clusterManagerDatabase, ti.openShiftClustersDatabase, ti.subscriptionsDatabase, nil, api.APIs, &noop.Noop{}, nil, nil, func(*logrus.Entry, env.Interface, *api.OpenShiftCluster) (adminactions.KubeActions, error) {
				return k, nil
			}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}

			go f.Run(ctx, nil, nil)

			resp, b, err := ti.request(method,
				fmt.Sprintf("https://server/admin%s/etcdrecovery?api-version=admin", resourceID),
				nil, nil)
				// tt.headers, []byte(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			err = validateResponse(resp, b, tt.wantStatusCode, tt.wantError, tt.wantResponse)
			if err != nil {
				t.Error(err)
			}
			if tt.wantResponseContentType != resp.Header.Get("Content-Type") {
				t.Error(fmt.Errorf("unexpected \"Content-Type\" response header value \"%s\", wanted \"%s\"", resp.Header.Get("Content-Type"), tt.wantResponseContentType))
			}
		})
	}
}
