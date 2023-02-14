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
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"time"
)

// ValueFromSecretReconciler reconciles a Secret object with ClusterSecret annotations
type ValueFromSecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *ValueFromSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	secret := &v1.Secret{}
	err := r.Get(ctx, req.NamespacedName, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Secret, requeuing the request")
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	isMarkedToBeDeleted := secret.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		return ctrl.Result{}, nil
	}

	var clusterSecretName string
	for k, v := range secret.Annotations {
		if k == "chistadata.io/name" {
			clusterSecretName = v
		}
	}
	if clusterSecretName == "" {
		log.V(2).Info("The secret doesn't have ClusterSecret annotation, skipping it")
		return ctrl.Result{}, nil
	}
	log.Info("Starting the update for the ClusterSecret, triggered by an update to the secret", "clusterSecret.Name", clusterSecretName)

	clusterSecret := &v12.ClusterSecret{}
	err = r.Get(ctx, types.NamespacedName{Name: clusterSecretName}, clusterSecret)
	if err != nil {
		log.Error(err, "Failed to get ClusterSecret, requeuing the request")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	if clusterSecret.Spec.ValueFrom.SecretName != secret.Name || clusterSecret.Spec.ValueFrom.SecretNamespace != secret.Namespace {
		log.Info("The secret is not defined in the ClusterSecret ValueFrom, please update the ClusterSecret resource. Requeing the request")
		return ctrl.Result{RequeueAfter: 2 * time.Minute}, err
	}
	if reflect.DeepEqual(clusterSecret.Spec.Data, secret.Data) && clusterSecret.Spec.Type == secret.Type {
		log.V(2).Info("The ClusterSecret Data and Type has not changed", "clusterSecret.Name", clusterSecret.Name)
		return ctrl.Result{}, nil
	}
	clusterSecret.Spec.Type = secret.Type
	clusterSecret.Spec.Data = secret.Data

	err = r.Update(ctx, clusterSecret)
	if err != nil {
		log.Error(err, "Failed to update ClusterSecret", "clusterSecret.Name", clusterSecret.Name)
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}
	log.Info("ClusterSecret updated successfully", "clusterSecret.Name", clusterSecret.Name)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ValueFromSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Secret{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(event event.CreateEvent) bool {
				// Check if annotation is present AND
				// add 5 seconds as the initial delay for this controller to start watching events to prevent double execution at startup
				return event.Object.GetAnnotations()["chistadata.io/name"] != "" && time.Now().Sub(event.Object.GetCreationTimestamp().Time) < 5*time.Second
			}}).
		Complete(r)
}
