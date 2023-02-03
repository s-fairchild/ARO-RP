package etcdfix

// Copyright (c) Microsoft Corporation.
// Licensed under the Apache License 2.0.

import (
	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	operatorclient "github.com/openshift/client-go/operator/clientset/versioned"
	securityclient "github.com/openshift/client-go/security/clientset/versioned"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	arov1alpha1 "github.com/Azure/ARO-RP/pkg/operator/apis/aro.openshift.io/v1alpha1"
	"github.com/Azure/ARO-RP/pkg/util/dynamichelper"
)

const (
	ControllerName = "etcdfixController"
)

type Reconciler struct {
	log *logrus.Entry

	dynamicHelper dynamichelper.Interface
	restConfig    *rest.Config
	securitycli   securityclient.Interface
	operatorcli operatorclient.Interface
	client 		client.Client
}

func NewReconciler(log *logrus.Entry, restConfig *rest.Config, securitycli securityclient.Interface) *Reconciler {
	return &Reconciler{
		log:         log,

		restConfig:  restConfig,
		securitycli: securitycli,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	instance := &arov1alpha1.Cluster{}
	err := r.client.Get(ctx, types.NamespacedName{Name: arov1alpha1.SingletonClusterName}, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	etcdConn := configv1.EtcdConnectionInfo{}
	err = r.client.Get(ctx, types.NamespacedName{Name: request.Name}, etcdConn)
	if err != nil {
		return reconcile.Result{}, err
	}
	r.log.Infof("Here's etcdConn struct: %v", etcdConn)
	// configs, err := r.operatorcli.OperatorV1().Configs().List(ctx, v1.ListOptions{})
	// sr, err := r.operatorcli.Discovery().ServerResources()

	// etcd, err := r.operatorcli.OperatorV1().Etcds().List(ctx, v1.ListOptions{})
	// etcds := &operatorv1.EtcdList{}
	// r.client.Get(ctx, types.NamespacedName{Name: request.Name}, etcds)
	// if err != nil {
	// 	return reconcile.Result{}, err
	// }
	r.log.Debug("running")
	// r.check(etcd, nil)

	// if err != nil {
	// 	r.log.Error(err)
	// 	return reconcile.Result{}, err
	// }

	// result, err := r.deploy(ctx, instance)
	// if err != nil {
	// 	r.log.Error(err)
	// 	return result, err
	// }

	return ctrl.Result{}, nil
}

func (r *Reconciler) check(etcds *operatorv1.EtcdList, etcdConn *configv1.EtcdConnectionInfo) (error) {
	for _, e := range etcds.Items {
		for _, i := range e.Status.Conditions {
			// end early if etcd status is false
			if i.Status != operatorv1.ConditionFalse {
				break
			}
			r.log.Info("etcd status: %s", i.Status)
			r.log.Info("etcd message: %s", i.Message)
			r.log.Info("etcd reason: %s", i.Reason)

			// os, err := e.Spec.StaticPodOperatorSpec.OperatorSpec.ObservedConfig.MarshalJSON()
			// if err != nil {
			// 	return err
			// }
			
			// d := e.Spec.StaticPodOperatorSpec.OperatorSpec.ObservedConfig.UnmarshalJSON()
		}
	}
	return nil
}

// func decodeTestDoc(r *http.Request, t *testing.T, doc interface{}) {
// 	if r.Body != http.NoBody {
// 		err := codec.NewDecoder(r.Body, &codec.JsonHandle{}).Decode(doc)
// 		if err != nil {
// 			t.Fatalf("\n%s\nfailed to decode document from request body\n%v\n", t.Name(), err)
// 		}
// 	}
// }

// func encodeTestDoc(w http.ResponseWriter, r *http.Request, t *testing.T, docs interface{}) {
// 	buf := &bytes.Buffer{}
// 	err := codec.NewEncoder(buf, &codec.JsonHandle{}).Encode(docs)
// 	if err != nil {
// 		t.Logf("\n%s\nfailed to encode document to request body\n%v\n", t.Name(), err)
// 	}
// 	w.Header().Set("Content-Type", "application/json")
// 	w.Header().Set("x-ms-version", "2018-12-31")
// 	w.Write(buf.Bytes())
// }

// func (r *Reconciler) deploy(ctx context.Context, instance *arov1alpha1.Cluster) (ctrl.Result, error) {
// 	var err error
// 	r.dynamicHelper, err = dynamichelper.New(r.log, r.restConfig)
// 	if err != nil {
// 		r.log.Error(err)
// 		return reconcile.Result{}, err
// 	}

// 	resources, err := r.resources(ctx, instance)
// 	if err != nil {
// 		r.log.Error(err)
// 		return reconcile.Result{}, err
// 	}

// 	err = dynamichelper.SetControllerReferences(resources, instance)
// 	if err != nil {
// 		r.log.Error(err)
// 		return reconcile.Result{}, err
// 	}

// 	err = dynamichelper.Prepare(resources)
// 	if err != nil {
// 		r.log.Error(err)
// 		return reconcile.Result{}, err
// 	}

// 	err = r.dynamicHelper.Ensure(ctx, resources...)
// 	if err != nil {
// 		r.log.Error(err)
// 		return reconcile.Result{}, err
// 	}

// 	return reconcile.Result{}, nil
// }

// SetupWithManager creates the controller
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	aroClusterPredicate := predicate.NewPredicateFuncs(func(o client.Object) bool {
		return o.GetName() == arov1alpha1.SingletonClusterName
	})

	defaultClusterETCDPredicate := predicate.NewPredicateFuncs(func(o client.Object) bool {
		return o.GetName() == "default"
	})

	filterEventPredicate := predicate.NewPredicateFuncs(func(o client.Object) bool {
		return o.GetName() == "update"
	})

	b := ctrl.NewControllerManagedBy(mgr).
		For(&arov1alpha1.Cluster{}, builder.WithPredicates(aroClusterPredicate)).
		WithEventFilter(filterEventPredicate).
		Watches(
			&source.Kind{Type: &operatorv1.Etcd{Spec: operatorv1.EtcdSpec{}}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(defaultClusterETCDPredicate),
		)

	return b.Named(ControllerName).Complete(r)
}

func (r *Reconciler) InjectClient(c client.Client) error {
	r.client = c
	return nil
}
