package frontend

// Copyright (c) Microsoft Corporation.
// Licensed under the Apache License 2.0.

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	operatorv1fake "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1/fake"
	"github.com/ugorji/go/codec"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktesting "k8s.io/client-go/testing"

	"github.com/Azure/ARO-RP/pkg/api"
	"github.com/Azure/ARO-RP/pkg/metrics/noop"
	mock_adminactions "github.com/Azure/ARO-RP/pkg/util/mocks/adminactions"
	testdatabase "github.com/Azure/ARO-RP/test/database"
)

const degradedNode = "master-2"

// TODO fix tests
func TestFixEtcd(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKey, "TRUE")

	const (
		mockSubID    = "00000000-0000-0000-0000-000000000000"
		mockTenantID = "00000000-0000-0000-0000-000000000000"
		urlParams    = "?api-version=admin&namespace=openshift-etcd&name=cluster&kind=Etcd"
	)
	resourceID := testdatabase.GetResourcePath(mockSubID, "cluster")
	doc := &api.OpenShiftClusterDocument{
		Key: strings.ToLower(resourceID),
		OpenShiftCluster: &api.OpenShiftCluster{
			Name: "cluster",
			ID:   resourceID,
			Type: "Microsoft.RedHatOpenShift/openshiftClusters",
			Properties: api.OpenShiftClusterProperties{
				InfraID: "zfsbk",
			},
		},
	}

	type test struct {
		name    string
		mocks   func(tt *test, t *testing.T, ti *testInfra, k *mock_adminactions.MockKubeActions, pods *corev1.PodList)
		wantErr string
		pods    *corev1.PodList
	}

	for _, tt := range []*test{
		{
			name:    "fail: list pods",
			wantErr: "oh no, can't list pods",
			mocks: func(tt *test, t *testing.T, ti *testInfra, k *mock_adminactions.MockKubeActions, pods *corev1.PodList) {
				k.EXPECT().KubeList(ctx, "Pod", namespaceEtcds).MaxTimes(1).Return(nil, errors.New("oh no, can't list pods"))
			},
		},
		{
			name:    "fail: invalid json, can't decode pods",
			wantErr: "json decode error [pos 1]: only encoded map or array can decode into struct",
			mocks: func(tt *test, t *testing.T, ti *testInfra, k *mock_adminactions.MockKubeActions, pods *corev1.PodList) {
				buf := &bytes.Buffer{}
				err := codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(`{`)
				if err != nil {
					t.Fatalf("failed to encode pods, %s", err.Error())
				}
				k.EXPECT().KubeList(ctx, "Pod", namespaceEtcds).MaxTimes(1).Return(buf.Bytes(), nil)
			},
		},
		{
			name: "pass: Expected degraded etcd scenario",
			pods: newDegradedPods(doc, false, false),
			mocks: func(tt *test, t *testing.T, ti *testInfra, k *mock_adminactions.MockKubeActions, pods *corev1.PodList) {
				buf := &bytes.Buffer{}
				err := codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(pods)
				if err != nil {
					t.Fatalf("%s failed to encode pods, %s", t.Name(), err.Error())
				}
				k.EXPECT().KubeList(ctx, "Pod", namespaceEtcds).MaxTimes(1).Return(buf.Bytes(), nil)

				// backupEtcd
				k.EXPECT().KubeCreateOrUpdate(ctx, createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))).MaxTimes(1).Return(nil)
				propPolicy := metav1.DeletePropagationBackground
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobNameDataBackup, true, &propPolicy).MaxTimes(1).Return(nil)

				// fixPeers
				// createPrivilegedServiceAccount
				serviceAcc := newServiceAccount(serviceAccountName, doc.OpenShiftCluster.Name)
				clusterRole := newClusterRole(kubeServiceAccount, doc.OpenShiftCluster.Name)
				crb := newClusterRoleBinding(serviceAccountName, doc.OpenShiftCluster.Name)
				scc := newSecurityContextConstraint(serviceAccountName, doc.OpenShiftCluster.Name, kubeServiceAccount)

				k.EXPECT().KubeCreateOrUpdate(ctx, serviceAcc).MaxTimes(1).Return(nil)
				k.EXPECT().KubeCreateOrUpdate(ctx, clusterRole).MaxTimes(1).Return(nil)
				k.EXPECT().KubeCreateOrUpdate(ctx, crb).MaxTimes(1).Return(nil)
				k.EXPECT().KubeCreateOrUpdate(ctx, scc).MaxTimes(1).Return(nil)

				de, err := comparePodEnvToIP(ti.log, pods)
				if err != nil {
					t.Fatal(err)
				}
				peerPods, err := getPeerPods(pods.Items, de, doc.OpenShiftCluster.Name)
				if err != nil {
					t.Fatal(err)
				}

				k.EXPECT().KubeCreateOrUpdate(ctx, newJobFixPeers(doc.OpenShiftCluster.Name, peerPods, de.Node)).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobNameFixPeers, true, &propPolicy).MaxTimes(1).Return(nil)

				// cleanup
				k.EXPECT().KubeDelete(ctx, serviceAcc.GetKind(), serviceAcc.GetNamespace(), serviceAcc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, scc.GetKind(), scc.GetNamespace(), scc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, clusterRole.GetKind(), clusterRole.GetNamespace(), clusterRole.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, crb.GetKind(), crb.GetNamespace(), crb.GetName(), true, nil).MaxTimes(1).Return(nil)

				err = codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(&operatorv1fake.FakeEtcds{})
				if err != nil {
					t.Fatal(err)
				}
				k.EXPECT().KubeGet(ctx, "Etcd", "", doc.OpenShiftCluster.Name).MaxTimes(1).Return(buf.Bytes(), nil)

				// delete secrets
				for _, prefix := range []string{"etcd-peer-", "etcd-serving-", "etcd-serving-metrics-"} {
					k.EXPECT().KubeDelete(ctx, "Secret", namespaceEtcds, prefix+buildNodeName(doc, degradedNode), false, nil)
				}
			},
		},
		{
			name: "pass: Empty env vars scenario",
			pods: newDegradedPods(doc, false, true),
			mocks: func(tt *test, t *testing.T, ti *testInfra, k *mock_adminactions.MockKubeActions, pods *corev1.PodList) {
				buf := &bytes.Buffer{}
				err := codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(pods)
				if err != nil {
					t.Fatalf("%s failed to encode pods, %s", t.Name(), err.Error())
				}
				k.EXPECT().KubeList(ctx, "Pod", namespaceEtcds).MaxTimes(1).Return(buf.Bytes(), nil)

				// backupEtcd
				k.EXPECT().KubeCreateOrUpdate(ctx, createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))).MaxTimes(1).Return(nil)
				propPolicy := metav1.DeletePropagationBackground
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobNameDataBackup, true, &propPolicy).MaxTimes(1).Return(nil)

				// fixPeers
				// createPrivilegedServiceAccount
				serviceAcc := newServiceAccount(serviceAccountName, doc.OpenShiftCluster.Name)
				clusterRole := newClusterRole(kubeServiceAccount, doc.OpenShiftCluster.Name)
				crb := newClusterRoleBinding(serviceAccountName, doc.OpenShiftCluster.Name)
				scc := newSecurityContextConstraint(serviceAccountName, doc.OpenShiftCluster.Name, kubeServiceAccount)

				k.EXPECT().KubeCreateOrUpdate(ctx, serviceAcc).MaxTimes(1).Return(nil)
				k.EXPECT().KubeCreateOrUpdate(ctx, clusterRole).MaxTimes(1).Return(nil)
				k.EXPECT().KubeCreateOrUpdate(ctx, crb).MaxTimes(1).Return(nil)
				k.EXPECT().KubeCreateOrUpdate(ctx, scc).MaxTimes(1).Return(nil)

				de, err := comparePodEnvToIP(ti.log, pods)
				if err != nil {
					t.Fatal(err)
				}
				peerPods, err := getPeerPods(pods.Items, de, doc.OpenShiftCluster.Name)
				if err != nil {
					t.Fatal(err)
				}

				k.EXPECT().KubeCreateOrUpdate(ctx, newJobFixPeers(doc.OpenShiftCluster.Name, peerPods, de.Node)).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobNameFixPeers, true, &propPolicy).MaxTimes(1).Return(nil)

				// cleanup
				k.EXPECT().KubeDelete(ctx, serviceAcc.GetKind(), serviceAcc.GetNamespace(), serviceAcc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, scc.GetKind(), scc.GetNamespace(), scc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, clusterRole.GetKind(), clusterRole.GetNamespace(), clusterRole.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, crb.GetKind(), crb.GetNamespace(), crb.GetName(), true, nil).MaxTimes(1).Return(nil)

				err = codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(&operatorv1fake.FakeEtcds{})
				if err != nil {
					t.Fatal(err)
				}
				k.EXPECT().KubeGet(ctx, "Etcd", "", doc.OpenShiftCluster.Name).MaxTimes(1).Return(buf.Bytes(), nil)

				// delete secrets
				for _, prefix := range []string{"etcd-peer-", "etcd-serving-", "etcd-serving-metrics-"} {
					k.EXPECT().KubeDelete(ctx, "Secret", namespaceEtcds, prefix+buildNodeName(doc, degradedNode), false, nil)
				}
			},
		},
		{
			name:    "fail: Multiple degraded etcd instances scenario",
			wantErr: "only a single degraded quorum is supported, more than one not Ready etcd pods were found: [etcd-cluster-zfsbk-master-2 etcd etcd]",
			pods:    newDegradedPods(doc, true, true),
			mocks: func(tt *test, t *testing.T, ti *testInfra, k *mock_adminactions.MockKubeActions, pods *corev1.PodList) {
				buf := &bytes.Buffer{}
				err := codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(pods)
				if err != nil {
					t.Fatalf("%s failed to encode pods, %s", t.Name(), err.Error())
				}
				k.EXPECT().KubeList(ctx, "Pod", namespaceEtcds).MaxTimes(1).Return(buf.Bytes(), nil)
			},
		},
		{
			name:    "fail: empty/correct pod env and no bad container statuses",
			wantErr: "no etcd pod's were found in a CrashLoopBackOff state, unable to remediate etcd deployment",
			pods:    &corev1.PodList{},
			mocks: func(tt *test, t *testing.T, ti *testInfra, k *mock_adminactions.MockKubeActions, pods *corev1.PodList) {
				buf := &bytes.Buffer{}
				err := codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(pods)
				if err != nil {
					t.Fatalf("%s failed to encode pods, %s", t.Name(), err.Error())
				}
				k.EXPECT().KubeList(ctx, "Pod", namespaceEtcds).MaxTimes(1).Return(buf.Bytes(), nil)
			},
		},
		{
			name:    "fail: create job data backup",
			wantErr: "oh no, can't create job data backup",
			pods:    newDegradedPods(doc, false, false),
			mocks: func(tt *test, t *testing.T, ti *testInfra, k *mock_adminactions.MockKubeActions, pods *corev1.PodList) {
				buf := &bytes.Buffer{}
				err := codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(pods)
				if err != nil {
					t.Fatalf("%s failed to encode pods, %s", t.Name(), err.Error())
				}
				k.EXPECT().KubeList(ctx, "Pod", namespaceEtcds).MaxTimes(1).Return(buf.Bytes(), nil)

				// backupEtcd
				k.EXPECT().KubeCreateOrUpdate(ctx, createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))).MaxTimes(1).Return(errors.New(tt.wantErr))
			},
		},
		{
			name:    "fail: create job fix peers",
			wantErr: "oh no, can't create job fix peers",
			pods:    newDegradedPods(doc, false, false),
			mocks: func(tt *test, t *testing.T, ti *testInfra, k *mock_adminactions.MockKubeActions, pods *corev1.PodList) {
				buf := &bytes.Buffer{}
				err := codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(pods)
				if err != nil {
					t.Fatalf("%s failed to encode pods, %s", t.Name(), err.Error())
				}
				k.EXPECT().KubeList(ctx, "Pod", namespaceEtcds).MaxTimes(1).Return(buf.Bytes(), nil)

				// backupEtcd
				k.EXPECT().KubeCreateOrUpdate(ctx, createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))).MaxTimes(1).Return(nil)
				propPolicy := metav1.DeletePropagationBackground
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobNameDataBackup, true, &propPolicy).MaxTimes(1).Return(nil)

				// fixPeers
				// createPrivilegedServiceAccount
				serviceAcc := newServiceAccount(serviceAccountName, doc.OpenShiftCluster.Name)
				clusterRole := newClusterRole(kubeServiceAccount, doc.OpenShiftCluster.Name)
				crb := newClusterRoleBinding(serviceAccountName, doc.OpenShiftCluster.Name)
				scc := newSecurityContextConstraint(serviceAccountName, doc.OpenShiftCluster.Name, kubeServiceAccount)

				k.EXPECT().KubeCreateOrUpdate(ctx, serviceAcc).MaxTimes(1).Return(nil)
				k.EXPECT().KubeCreateOrUpdate(ctx, clusterRole).MaxTimes(1).Return(nil)
				k.EXPECT().KubeCreateOrUpdate(ctx, crb).MaxTimes(1).Return(nil)
				k.EXPECT().KubeCreateOrUpdate(ctx, scc).MaxTimes(1).Return(nil)

				de, err := comparePodEnvToIP(ti.log, pods)
				if err != nil {
					t.Fatal(err)
				}
				peerPods, err := getPeerPods(pods.Items, de, doc.OpenShiftCluster.Name)
				if err != nil {
					t.Fatal(err)
				}

				k.EXPECT().KubeCreateOrUpdate(ctx, newJobFixPeers(doc.OpenShiftCluster.Name, peerPods, de.Node)).MaxTimes(1).Return(errors.New("oh no, can't create job fix peers"))

				// cleanup
				k.EXPECT().KubeDelete(ctx, serviceAcc.GetKind(), serviceAcc.GetNamespace(), serviceAcc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, scc.GetKind(), scc.GetNamespace(), scc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, clusterRole.GetKind(), clusterRole.GetNamespace(), clusterRole.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, crb.GetKind(), crb.GetNamespace(), crb.GetName(), true, nil).MaxTimes(1).Return(nil)
			},
		},
		{
			name:    "fail: create service account",
			wantErr: "oh no, can't create service account %!s(<nil>)",
			pods:    newDegradedPods(doc, false, false),
			mocks: func(tt *test, t *testing.T, ti *testInfra, k *mock_adminactions.MockKubeActions, pods *corev1.PodList) {
				buf := &bytes.Buffer{}
				err := codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(pods)
				if err != nil {
					t.Fatalf("%s failed to encode pods, %s", t.Name(), err.Error())
				}
				k.EXPECT().KubeList(ctx, "Pod", namespaceEtcds).MaxTimes(1).Return(buf.Bytes(), nil)

				// backupEtcd
				k.EXPECT().KubeCreateOrUpdate(ctx, createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))).MaxTimes(1).Return(nil)

				// fixPeers
				serviceAcc := newServiceAccount(serviceAccountName, doc.OpenShiftCluster.Name)

				// k.EXPECT().KubeCreateOrUpdate(ctx, serviceAcc).MaxTimes(1).Return(errors.New(tt.wantErr))
				k.EXPECT().KubeCreateOrUpdate(ctx, serviceAcc).MaxTimes(1).Return(errors.New("oh no, can't create service account"))

				// nested cleanup
				propPolicy := metav1.DeletePropagationBackground
				k.EXPECT().KubeDelete(ctx, "ServiceAccount", namespaceEtcds, serviceAcc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "SecurityContextConstraints", "", serviceAcc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "ClusterRole", "", "system:serviceaccountopenshift-etcd:etcd-recovery-privileged", true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "ClusterRoleBinding", "", serviceAcc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobNameDataBackup, true, &propPolicy).MaxTimes(1).Return(nil)
			},
		},
		{
			name:    "fail: create cluster role",
			wantErr: "oh no, can't create job fix peers %!s(<nil>)",
			pods:    newDegradedPods(doc, false, false),
			mocks: func(tt *test, t *testing.T, ti *testInfra, k *mock_adminactions.MockKubeActions, pods *corev1.PodList) {
				buf := &bytes.Buffer{}
				err := codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(pods)
				if err != nil {
					t.Fatalf("%s failed to encode pods, %s", t.Name(), err.Error())
				}
				k.EXPECT().KubeList(ctx, "Pod", namespaceEtcds).MaxTimes(1).Return(buf.Bytes(), nil)

				// backupEtcd
				k.EXPECT().KubeCreateOrUpdate(ctx, createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))).MaxTimes(1).Return(nil)
				propPolicy := metav1.DeletePropagationBackground
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobNameDataBackup, true, &propPolicy).MaxTimes(1).Return(nil)

				// fixPeers
				// createPrivilegedServiceAccount
				serviceAcc := newServiceAccount(serviceAccountName, doc.OpenShiftCluster.Name)
				clusterRole := newClusterRole(kubeServiceAccount, doc.OpenShiftCluster.Name)

				k.EXPECT().KubeCreateOrUpdate(ctx, serviceAcc).MaxTimes(1).Return(nil)
				k.EXPECT().KubeCreateOrUpdate(ctx, clusterRole).MaxTimes(1).Return(errors.New("oh no, can't create job fix peers"))
				k.EXPECT().KubeDelete(ctx, "ServiceAccount", namespaceEtcds, serviceAcc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "SecurityContextConstraints", "", serviceAcc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "ClusterRole", "", "system:serviceaccountopenshift-etcd:etcd-recovery-privileged", true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "ClusterRoleBinding", "", serviceAcc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobNameDataBackup, true, &propPolicy).MaxTimes(1).Return(nil)
			},
		},
		{
			name:    "fail: create cluster role binding",
			wantErr: "oh no, can't create cluster role binding %!s(<nil>)",
			pods:    newDegradedPods(doc, false, false),
			mocks: func(tt *test, t *testing.T, ti *testInfra, k *mock_adminactions.MockKubeActions, pods *corev1.PodList) {
				buf := &bytes.Buffer{}
				err := codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(pods)
				if err != nil {
					t.Fatalf("%s failed to encode pods, %s", t.Name(), err.Error())
				}
				k.EXPECT().KubeList(ctx, "Pod", namespaceEtcds).MaxTimes(1).Return(buf.Bytes(), nil)

				// backupEtcd
				k.EXPECT().KubeCreateOrUpdate(ctx, createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))).MaxTimes(1).Return(nil)
				propPolicy := metav1.DeletePropagationBackground
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobNameDataBackup, true, &propPolicy).MaxTimes(1).Return(nil)

				// fixPeers
				// createPrivilegedServiceAccount
				serviceAcc := newServiceAccount(serviceAccountName, doc.OpenShiftCluster.Name)
				clusterRole := newClusterRole(kubeServiceAccount, doc.OpenShiftCluster.Name)
				crb := newClusterRoleBinding(serviceAccountName, doc.OpenShiftCluster.Name)

				// cleanup
				k.EXPECT().KubeCreateOrUpdate(ctx, serviceAcc).MaxTimes(1).Return(nil)
				k.EXPECT().KubeCreateOrUpdate(ctx, clusterRole).MaxTimes(1).Return(nil)
				k.EXPECT().KubeCreateOrUpdate(ctx, crb).MaxTimes(1).Return(errors.New("oh no, can't create cluster role binding"))
				k.EXPECT().KubeDelete(ctx, "ServiceAccount", namespaceEtcds, serviceAcc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "SecurityContextConstraints", "", serviceAcc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "ClusterRole", "", "system:serviceaccountopenshift-etcd:etcd-recovery-privileged", true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "ClusterRoleBinding", "", serviceAcc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobNameDataBackup, true, &propPolicy).MaxTimes(1).Return(nil)
			},
		},
		{
			name:    "fail: create security context constraint",
			wantErr: "oh no, can't create security context constraint %!s(<nil>)",
			pods:    newDegradedPods(doc, false, false),
			mocks: func(tt *test, t *testing.T, ti *testInfra, k *mock_adminactions.MockKubeActions, pods *corev1.PodList) {
				buf := &bytes.Buffer{}
				err := codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(pods)
				if err != nil {
					t.Fatalf("%s failed to encode pods, %s", t.Name(), err.Error())
				}
				k.EXPECT().KubeList(ctx, "Pod", namespaceEtcds).MaxTimes(1).Return(buf.Bytes(), nil)

				// backupEtcd
				k.EXPECT().KubeCreateOrUpdate(ctx, createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))).MaxTimes(1).Return(nil)
				propPolicy := metav1.DeletePropagationBackground
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobNameDataBackup, true, &propPolicy).MaxTimes(1).Return(nil)

				// fixPeers
				// createPrivilegedServiceAccount
				serviceAcc := newServiceAccount(serviceAccountName, doc.OpenShiftCluster.Name)
				clusterRole := newClusterRole(kubeServiceAccount, doc.OpenShiftCluster.Name)
				crb := newClusterRoleBinding(serviceAccountName, doc.OpenShiftCluster.Name)
				scc := newSecurityContextConstraint(serviceAccountName, doc.OpenShiftCluster.Name, kubeServiceAccount)

				k.EXPECT().KubeCreateOrUpdate(ctx, serviceAcc).MaxTimes(1).Return(nil)
				k.EXPECT().KubeCreateOrUpdate(ctx, clusterRole).MaxTimes(1).Return(nil)
				k.EXPECT().KubeCreateOrUpdate(ctx, crb).MaxTimes(1).Return(nil)
				k.EXPECT().KubeCreateOrUpdate(ctx, scc).MaxTimes(1).Return(errors.New("oh no, can't create security context constraint"))

				// cleanup
				k.EXPECT().KubeDelete(ctx, "ServiceAccount", namespaceEtcds, serviceAcc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "SecurityContextConstraints", "", serviceAcc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "ClusterRole", "", "system:serviceaccountopenshift-etcd:etcd-recovery-privileged", true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "ClusterRoleBinding", "", serviceAcc.GetName(), true, nil).MaxTimes(1).Return(nil)
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobNameDataBackup, true, &propPolicy).MaxTimes(1).Return(nil)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ti := newTestInfra(t).WithOpenShiftClusters().WithSubscriptions()
			defer ti.done()

			k := mock_adminactions.NewMockKubeActions(ti.controller)
			tt.mocks(tt, t, ti, k, tt.pods)

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

			f, err := NewFrontend(ctx,
				ti.audit,
				ti.log,
				ti.env,
				ti.asyncOperationsDatabase,
				ti.clusterManagerDatabase,
				ti.openShiftClustersDatabase,
				ti.subscriptionsDatabase,
				nil,
				api.APIs,
				&noop.Noop{},
				nil,
				nil,
				nil,
				nil,
				ti.enricher)
			if err != nil {
				t.Fatal(err)
			}

			err = f.fixEtcd(ctx, ti.log, ti.env, doc, k, &operatorv1fake.FakeEtcds{
				Fake: &operatorv1fake.FakeOperatorV1{
					Fake: &ktesting.Fake{},
				},
			})
			if err != nil && err.Error() != tt.wantErr ||
				err == nil && tt.wantErr != "" {
				t.Errorf("\n%s\n !=\n%s", err.Error(), tt.wantErr)
			}
		})
	}
}

func buildClusterName(doc *api.OpenShiftClusterDocument) string {
	return doc.OpenShiftCluster.Name + "-" + doc.OpenShiftCluster.Properties.InfraID
}

func buildNodeName(doc *api.OpenShiftClusterDocument, node string) string {
	c := buildClusterName(doc)
	return c + "-" + node
}

func newDegradedPods(doc *api.OpenShiftClusterDocument, multiDegraded, emptyEnv bool) *corev1.PodList {
	var (
		nodeName  = buildNodeName(doc, degradedNode)
		master0IP = "10.0.0.1"
		master1IP = "10.0.0.2"
		master2IP = "10.0.0.3"
	)
	envs := []corev1.EnvVar{
		{
			Name:  "NODE_" + doc.OpenShiftCluster.Name + "_" + doc.OpenShiftCluster.Properties.InfraID + "_master_0_IP",
			Value: master0IP,
		},
		{
			Name:  "NODE_ " + doc.OpenShiftCluster.Name + "_" + doc.OpenShiftCluster.Properties.InfraID + "_master_1_IP",
			Value: master1IP,
		},
		{
			// degraded pod
			Name:  "NODE_" + doc.OpenShiftCluster.Name + "_" + doc.OpenShiftCluster.Properties.InfraID + "_master_2_IP",
			Value: master2IP,
		},
	}

	// Used to test scenario when etcd's env vars are empty, or there is no conflict found
	// then statuses will be tests
	if emptyEnv {
		envs = []corev1.EnvVar{}
	}
	badStatus := []corev1.ContainerStatus{
		{
			Name:         "etcd",
			Ready:        false,
			Started:      to.BoolPtr(false),
			RestartCount: 50,
			State: corev1.ContainerState{
				Waiting: &corev1.ContainerStateWaiting{
					Reason:  "Container is in a crashloop backoff",
					Message: "Container crashloop backoff",
				},
			},
		},
	}

	etcd2Statuses := []corev1.ContainerStatus{}
	if multiDegraded {
		etcd2Statuses = badStatus
	}

	return &corev1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind: "Etcd",
		},
		Items: []corev1.Pod{
			// IP mismatch container
			{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "etcd-" + nodeName,
					Namespace: namespaceEtcds,
				},
				Status: corev1.PodStatus{
					ContainerStatuses: badStatus,
					PodIPs: []corev1.PodIP{
						{
							IP: master0IP,
						},
						{
							IP: master1IP,
						},
						{
							IP: master2IP,
						},
					},
				},
				Spec: corev1.PodSpec{
					NodeName: nodeName,
					Containers: []corev1.Container{
						{
							Name: "etcd",
							Env:  envs,
						},
					},
				},
			},
			{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "etcd",
					Namespace: namespaceEtcds,
				},
				Status: corev1.PodStatus{
					ContainerStatuses: etcd2Statuses,
				},
				Spec: corev1.PodSpec{
					NodeName: "master-1",
					// healthy container
					Containers: []corev1.Container{
						{
							Name: "etcd",
							Env:  envs,
						},
					},
				},
			},
			{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name: "etcd",
				},
				Status: corev1.PodStatus{
					ContainerStatuses: etcd2Statuses,
				},
				Spec: corev1.PodSpec{
					NodeName: "master-0",
					// healthy container
					Containers: []corev1.Container{
						{
							Name: "etcd",
							Env:  envs,
						},
					},
				},
			},
		},
	}
}
