/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	v12 "github.com/ChistaDATA/cluster-secret-operator/api/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"time"
)

// NamespacesReconciler reconciles a ClusterSecret object when new namespaces are created
type NamespacesReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups=chistadata.io,resources=clustersecrets,verbs=get;list;watch;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *NamespacesReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	namespace := &v1.Namespace{}
	err := r.Get(ctx, req.NamespacedName, namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Namespace not found. Ignoring since object must be deleted", "namespace", namespace.Name)
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Namespace, requeuing the request", "namespace", namespace.Name)
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	isMarkedToBeDeleted := namespace.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		return ctrl.Result{}, nil
	}

	clusterSecrets := &v12.ClusterSecretList{}
	err = r.List(ctx, clusterSecrets)
	if err != nil {
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	for _, c := range clusterSecrets.Items {
		now := time.Now()
		if c.Annotations["chistadata.io/new-namespace-created-at"] != "" {
			clusterSecretLastUpdateFromNamespace, err := time.Parse(time.RFC3339, c.Annotations["chistadata.io/new-namespace-created-at"])
			if err != nil {
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
			if now.Sub(clusterSecretLastUpdateFromNamespace) < time.Minute {
				log.V(2).Info("The ClusterSecret was already updated by another namespace change", "clusterSecret.Name", c.Name)
				continue
			}
		}
		c.Annotations["chistadata.io/new-namespace-created-at"] = now.Format(time.RFC3339)

		err = r.Update(ctx, &c)
		if err != nil {
			log.Error(err, "Failed to update ClusterSecret", "clusterSecret.Name", c.Name)
			return ctrl.Result{RequeueAfter: time.Minute}, err
		}
		log.Info("ClusterSecret updated successfully", "clusterSecret.Name", c.Name)
		time.Sleep(2 * time.Second)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespacesReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Namespace{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(event event.CreateEvent) bool {
				// Add 5 seconds as the initial delay for this controller to start watching events to prevent double execution at startup
				return time.Now().Sub(event.Object.GetCreationTimestamp().Time) < 5*time.Second
			}}).
		Complete(r)
}
