package frontend

// Copyright (c) Microsoft Corporation.
// Licensed under the Apache License 2.0.

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Azure/go-autorest/autorest/to"
	operatorv1fake "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1/fake"
	"github.com/ugorji/go/codec"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	ktesting "k8s.io/client-go/testing"

	"github.com/Azure/ARO-RP/pkg/api"
	"github.com/Azure/ARO-RP/pkg/metrics/noop"
	mock_adminactions "github.com/Azure/ARO-RP/pkg/util/mocks/adminactions"
	testdatabase "github.com/Azure/ARO-RP/test/database"
)

const degradedNode = "master-2"

func TestFixEtcd(t *testing.T) {
	ctx := context.Background()
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
				jobBackupEtcd := createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))
				k.EXPECT().KubeCreateOrUpdate(ctx, jobBackupEtcd).MaxTimes(1).Return(nil)

				createPodCompletedEvent(ctx, jobBackupEtcd, k, "app")

				k.EXPECT().KubeGetPodLogs(ctx, jobBackupEtcd.GetNamespace(), jobBackupEtcd.GetName(), jobBackupEtcd.GetName()).MaxTimes(1).Return([]byte("Backup job doing backup things..."), nil)

				propPolicy := metav1.DeletePropagationBackground
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobBackupEtcd.GetName(), true, &propPolicy).MaxTimes(1).Return(nil)

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

				de, err := findDegradedEtcd(ti.log, pods)
				if err != nil {
					t.Fatal(err)
				}
				peerPods, err := getPeerPods(pods.Items, de, doc.OpenShiftCluster.Name)
				if err != nil {
					t.Fatal(err)
				}

				jobFixPeers := newJobFixPeers(doc.OpenShiftCluster.Name, peerPods, de.Node)
				k.EXPECT().KubeCreateOrUpdate(ctx, jobFixPeers).MaxTimes(1).Return(nil)
				createPodCompletedEvent(ctx, jobFixPeers, k, "app")

				k.EXPECT().KubeGetPodLogs(ctx, jobFixPeers.GetNamespace(), jobFixPeers.GetName(), jobFixPeers.GetName()).MaxTimes(1).Return([]byte("Fix peer job fixing peers..."), nil)
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobFixPeers.GetName(), true, &propPolicy).MaxTimes(1).Return(nil)

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
				jobBackupEtcd := createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))
				k.EXPECT().KubeCreateOrUpdate(ctx, jobBackupEtcd).MaxTimes(1).Return(nil)
				createPodCompletedEvent(ctx, jobBackupEtcd, k, "app")
				k.EXPECT().KubeGetPodLogs(ctx, jobBackupEtcd.GetNamespace(), jobBackupEtcd.GetName(), jobBackupEtcd.GetName()).MaxTimes(1).Return([]byte("Backup job doing backup things..."), nil)
				propPolicy := metav1.DeletePropagationBackground
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobBackupEtcd.GetName(), true, &propPolicy).MaxTimes(1).Return(nil)

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

				de, err := findDegradedEtcd(ti.log, pods)
				if err != nil {
					t.Fatal(err)
				}
				peerPods, err := getPeerPods(pods.Items, de, doc.OpenShiftCluster.Name)
				if err != nil {
					t.Fatal(err)
				}

				jobFixPeers := newJobFixPeers(doc.OpenShiftCluster.Name, peerPods, de.Node)
				k.EXPECT().KubeCreateOrUpdate(ctx, jobFixPeers).MaxTimes(1).Return(nil)
				createPodCompletedEvent(ctx, jobFixPeers, k, "app")
				k.EXPECT().KubeGetPodLogs(ctx, jobFixPeers.GetNamespace(), jobFixPeers.GetName(), jobFixPeers.GetName()).MaxTimes(1).Return([]byte("Fix peer job fixing peers..."), nil)
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobFixPeers.GetName(), true, &propPolicy).MaxTimes(1).Return(nil)

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
			wantErr: "only a single degraded etcd pod can can be recovered from, more than one NotReady etcd pods were found: [etcd-cluster-zfsbk-master-0 etcd-cluster-zfsbk-master-1 etcd-cluster-zfsbk-master-2]",
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
				jobBackupEtcd := createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))
				k.EXPECT().KubeCreateOrUpdate(ctx, jobBackupEtcd).MaxTimes(1).Return(errors.New(tt.wantErr))
				createPodCompletedEvent(ctx, jobBackupEtcd, k, "app")
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
				jobBackupEtcd := createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))
				k.EXPECT().KubeCreateOrUpdate(ctx, jobBackupEtcd).MaxTimes(1).Return(nil)
				createPodCompletedEvent(ctx, jobBackupEtcd, k, "app")
				k.EXPECT().KubeGetPodLogs(ctx, jobBackupEtcd.GetNamespace(), jobBackupEtcd.GetName(), jobBackupEtcd.GetName()).MaxTimes(1).Return([]byte("Backup job doing backup things..."), nil)
				propPolicy := metav1.DeletePropagationBackground
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobBackupEtcd.GetName(), true, &propPolicy).MaxTimes(1).Return(nil)

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

				de, err := findDegradedEtcd(ti.log, pods)
				if err != nil {
					t.Fatal(err)
				}
				peerPods, err := getPeerPods(pods.Items, de, doc.OpenShiftCluster.Name)
				if err != nil {
					t.Fatal(err)
				}

				jobFixPeers := newJobFixPeers(doc.OpenShiftCluster.Name, peerPods, de.Node)
				k.EXPECT().KubeCreateOrUpdate(ctx, jobFixPeers).MaxTimes(1).Return(errors.New("oh no, can't create job fix peers"))
				createPodCompletedEvent(ctx, jobFixPeers, k, "app")

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
				jobBackupEtcd := createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))
				k.EXPECT().KubeCreateOrUpdate(ctx, jobBackupEtcd).MaxTimes(1).Return(nil)
				createPodCompletedEvent(ctx, jobBackupEtcd, k, "app")
				k.EXPECT().KubeGetPodLogs(ctx, jobBackupEtcd.GetNamespace(), jobBackupEtcd.GetName(), jobBackupEtcd.GetName()).MaxTimes(1).Return([]byte("Backup job doing backup things..."), nil)

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
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobBackupEtcd.GetName(), true, &propPolicy).MaxTimes(1).Return(nil)
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
				jobBackupEtcd := createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))
				k.EXPECT().KubeCreateOrUpdate(ctx, jobBackupEtcd).MaxTimes(1).Return(nil)
				createPodCompletedEvent(ctx, jobBackupEtcd, k, "app")
				k.EXPECT().KubeGetPodLogs(ctx, jobBackupEtcd.GetNamespace(), jobBackupEtcd.GetName(), jobBackupEtcd.GetName()).MaxTimes(1).Return([]byte("Backup job doing backup things..."), nil)
				propPolicy := metav1.DeletePropagationBackground
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobBackupEtcd.GetName(), true, &propPolicy).MaxTimes(1).Return(nil)

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
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobBackupEtcd.GetName(), true, &propPolicy).MaxTimes(1).Return(nil)
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
				jobBackupEtcd := createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))
				k.EXPECT().KubeCreateOrUpdate(ctx, jobBackupEtcd).MaxTimes(1).Return(nil)
				createPodCompletedEvent(ctx, jobBackupEtcd, k, "app")
				k.EXPECT().KubeGetPodLogs(ctx, jobBackupEtcd.GetNamespace(), jobBackupEtcd.GetName(), jobBackupEtcd.GetName()).MaxTimes(1).Return([]byte("Backup job doing backup things..."), nil)
				propPolicy := metav1.DeletePropagationBackground
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobBackupEtcd.GetName(), true, &propPolicy).MaxTimes(1).Return(nil)

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
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobBackupEtcd.GetName(), true, &propPolicy).MaxTimes(1).Return(nil)
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
				jobBackupEtcd := createBackupEtcdDataJob(doc.OpenShiftCluster.Name, buildNodeName(doc, degradedNode))
				k.EXPECT().KubeCreateOrUpdate(ctx, jobBackupEtcd).MaxTimes(1).Return(nil)
				createPodCompletedEvent(ctx, jobBackupEtcd, k, "app")
				k.EXPECT().KubeGetPodLogs(ctx, jobBackupEtcd.GetNamespace(), jobBackupEtcd.GetName(), jobBackupEtcd.GetName()).MaxTimes(1).Return([]byte("Backup job doing backup things..."), nil)
				propPolicy := metav1.DeletePropagationBackground
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobBackupEtcd.GetName(), true, &propPolicy).MaxTimes(1).Return(nil)

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
				k.EXPECT().KubeDelete(ctx, "Job", namespaceEtcds, jobBackupEtcd.GetName(), true, &propPolicy).MaxTimes(1).Return(nil)
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

			containerLogs, err := f.fixEtcd(ctx, ti.log, ti.env, doc, k, &operatorv1fake.FakeEtcds{
				Fake: &operatorv1fake.FakeOperatorV1{
					Fake: &ktesting.Fake{},
				},
			})
			ti.log.Infof("Container logs: \n%s", containerLogs)
			if err != nil && err.Error() != tt.wantErr ||
				err == nil && tt.wantErr != "" {
				t.Errorf("\n%s\n !=\n%s", err.Error(), tt.wantErr)
			}
		})
	}
}

func createPodCompletedEvent(ctx context.Context, o *unstructured.Unstructured, k *mock_adminactions.MockKubeActions, labelKey string) {
	k.EXPECT().KubeWatch(ctx, o, labelKey).AnyTimes().DoAndReturn(func(ctx context.Context, o *unstructured.Unstructured, labelKey string) (watch.Interface, error) {
		w := watch.NewFake()
		go func() {
			time.Sleep(time.Microsecond)
			w.Action(watch.Added, newCompletedPodWatchEvent(o))
		}()
		return w, nil
	})
}

func newCompletedPodWatchEvent(o *unstructured.Unstructured) kruntime.Object {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.GetName(),
			Namespace: o.GetNamespace(),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
		},
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
		degradedNodeMaster2 = buildNodeName(doc, degradedNode)
		nodeMaster0         = buildNodeName(doc, "master-0")
		nodeMaster1         = buildNodeName(doc, "master-1")
	)
	const (
		master0IP        = "10.0.0.1"
		master1IP        = "10.0.0.2"
		master2IP        = "10.0.0.3"
		master2ChangedIP = "10.0.0.9"
	)

	// Used to test scenario when etcd's env vars are empty, or there is no conflict found
	// then statuses will be tests
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
			Name:  "NODE_" + doc.OpenShiftCluster.Name + "_" + doc.OpenShiftCluster.Properties.InfraID + "_master_2_IP",
			Value: master2IP,
		},
	}
	if emptyEnv {
		envs = []corev1.EnvVar{}
	}
	containerID := "quay://etcd-container-id"
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
			ContainerID: containerID,
		},
	}

	statuses := []corev1.ContainerStatus{
		{
			State:       corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			ContainerID: containerID,
		},
	}
	if multiDegraded {
		statuses = badStatus
	}

	return &corev1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind: "Etcd",
		},
		Items: []corev1.Pod{
			// healthy pod
			{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "etcd-" + nodeMaster0,
					Namespace: namespaceEtcds,
				},
				Status: corev1.PodStatus{
					ContainerStatuses: statuses,
					PodIPs: []corev1.PodIP{
						{
							IP: master0IP,
						},
					},
				},
				Spec: corev1.PodSpec{
					NodeName: nodeMaster0,
					Containers: []corev1.Container{
						{
							Name: "etcd",
							Env:  envs,
						},
					},
				},
			},
			// healthy pod
			{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "etcd-" + nodeMaster1,
					Namespace: namespaceEtcds,
				},
				Status: corev1.PodStatus{
					ContainerStatuses: statuses,
					PodIPs: []corev1.PodIP{
						{
							IP: master1IP,
						},
					},
				},
				Spec: corev1.PodSpec{
					NodeName: nodeMaster1,
					Containers: []corev1.Container{
						{
							Name: "etcd",
							Env:  envs,
						},
					},
				},
			},
			// degraded pod
			{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "etcd-" + degradedNodeMaster2,
					Namespace: namespaceEtcds,
				},
				Status: corev1.PodStatus{
					ContainerStatuses: badStatus,
					PodIPs: []corev1.PodIP{
						{
							IP: master2ChangedIP,
						},
					},
				},
				Spec: corev1.PodSpec{
					NodeName: degradedNodeMaster2,
					Containers: []corev1.Container{
						{
							Name: "etcd",
							Env:  envs,
						},
					},
				},
			},
		}}
}
