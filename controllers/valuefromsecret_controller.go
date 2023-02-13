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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
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
	_ = log.FromContext(ctx)

	secret := &v1.Secret{}
	err := r.Get(ctx, req.NamespacedName, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Log.Info("Secret resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Log.Error(err, "Failed to get Secret, requeuing the request")
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
		log.Log.Info("The secret %s/%s doesn't have ClusterSecret annotation, skipping it", secret.Namespace, secret.Name)
		return ctrl.Result{}, nil
	}
	log.Log.Info("Starting the update for the ClusterSecret %s, triggered by an update to the secret %s/%s", clusterSecretName, secret.Namespace, secret.Name)

	clusterSecret := &v12.ClusterSecret{}
	err = r.Get(ctx, types.NamespacedName{Name: clusterSecretName}, clusterSecret)
	if err != nil {
		log.Log.Error(err, "Failed to get ClusterSecret %s", clusterSecretName)
		return ctrl.Result{}, err
	}
	if clusterSecret.Spec.ValueFrom.SecretName != secret.Name || clusterSecret.Spec.ValueFrom.SecretNamespace != secret.Namespace {
		log.Log.Info("The secret %s/%s is not defined in the ClusterSecret %s ValueFrom, please update the ClusterSecret resource. Requeing the request", secret.Namespace, secret.Name, clusterSecretName)
		return ctrl.Result{RequeueAfter: 2 * time.Minute}, err
	}
	clusterSecret.Spec.Data = secret.Data

	err = r.Update(ctx, clusterSecret)
	if err != nil {
		log.Log.Error(err, "Failed to update ClusterSecret %s", clusterSecretName)
		return ctrl.Result{}, err
	}
	log.Log.Info("ClusterSecret %s updated successfully", clusterSecretName)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ValueFromSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Secret{}).
		Complete(r)
}
