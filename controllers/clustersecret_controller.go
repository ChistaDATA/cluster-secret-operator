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
	"fmt"
	v12 "github.com/ChistaDATA/cluster-secret-operator/api/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
	"time"
)

// Definitions to manage status conditions
const (
	// typeAvailableClusterSecret represents the status of the Deployment reconciliation
	typeAvailableClusterSecret = "Available"
)

// ClusterSecretReconciler reconciles a ClusterSecret object
type ClusterSecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=chistadata.io,resources=clustersecrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=chistadata.io,resources=clustersecrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=chistadata.io,resources=clustersecrets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *ClusterSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	clusterSecret := &v12.ClusterSecret{}
	err := r.Get(ctx, req.NamespacedName, clusterSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Log.Info("ClusterSecret resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Log.Error(err, "Failed to get ClusterSecret, requeuing the request")
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	if clusterSecret.Status.Conditions == nil || len(clusterSecret.Status.Conditions) == 0 {
		meta.SetStatusCondition(&clusterSecret.Status.Conditions, metav1.Condition{Type: typeAvailableClusterSecret, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, clusterSecret); err != nil {
			log.Log.Error(err, "Failed to update ClusterSecret status")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, clusterSecret); err != nil {
			log.Log.Error(err, "Failed to re-fetch ClusterSecret")
			return ctrl.Result{}, err
		}
	}

	isMarkedToBeDeleted := clusterSecret.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		return ctrl.Result{}, nil
	}

	ls := getLabels(clusterSecret.Name)

	k8sNamespaceList := &v1.NamespaceList{}
	err = r.List(ctx, k8sNamespaceList)
	if err != nil {
		meta.SetStatusCondition(&clusterSecret.Status.Conditions, metav1.Condition{Type: typeAvailableClusterSecret,
			Status: metav1.ConditionFalse, Reason: "Reconciling",
			Message: fmt.Sprintf("Failed to get namespace list: (%s)", err)})
		if err = r.Status().Update(ctx, clusterSecret); err != nil {
			log.Log.Error(err, "Failed to update ClusterSecret status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}
	namespaceList, err := r.filterNamespaceList(k8sNamespaceList, clusterSecret)
	if err != nil {
		meta.SetStatusCondition(&clusterSecret.Status.Conditions, metav1.Condition{Type: typeAvailableClusterSecret,
			Status: metav1.ConditionFalse, Reason: "Reconciling",
			Message: fmt.Sprintf("Failed to filter namespace list: (%s)", err)})
		if err = r.Status().Update(ctx, clusterSecret); err != nil {
			log.Log.Error(err, "Failed to update ClusterSecret status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	if clusterSecret.Spec.ValueFrom.SecretName == "" {
		if clusterSecret.Spec.Data == nil {
			err = fmt.Errorf("the Data must be provided when the ValueFrom is not provided")
			log.Log.Error(err, err.Error())

			meta.SetStatusCondition(&clusterSecret.Status.Conditions, metav1.Condition{Type: typeAvailableClusterSecret,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to filter namespace list: (%s)", err)})
			if err = r.Status().Update(ctx, clusterSecret); err != nil {
				log.Log.Error(err, "Failed to update ClusterSecret status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		_, err = r.createNewSecrets(ctx, clusterSecret, ls, namespaceList)
		if err != nil {
			meta.SetStatusCondition(&clusterSecret.Status.Conditions, metav1.Condition{Type: typeAvailableClusterSecret,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Secret: (%s)", err)})
			if err = r.Status().Update(ctx, clusterSecret); err != nil {
				log.Log.Error(err, "Failed to update ClusterSecret status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
	} else {
		source := &v1.Secret{}
		err = r.Get(ctx, types.NamespacedName{
			Name:      clusterSecret.Spec.ValueFrom.SecretName,
			Namespace: clusterSecret.Spec.ValueFrom.SecretNamespace,
		}, source)
		if err != nil {
			if apierrors.IsNotFound(err) {
				err = fmt.Errorf("the secret in the ValueFrom could not be found for the ClusterSecret %s", clusterSecret.Name)
				log.Log.Error(err, err.Error())

				meta.SetStatusCondition(&clusterSecret.Status.Conditions, metav1.Condition{Type: typeAvailableClusterSecret,
					Status: metav1.ConditionFalse, Reason: "Reconciling",
					Message: "The Secret in the ValueFrom could not be found"})
				if err = r.Status().Update(ctx, clusterSecret); err != nil {
					log.Log.Error(err, "Failed to update ClusterSecret status")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		clusterSecret.Spec.Type = source.Type
		clusterSecret.Spec.Data = source.Data
		_, err = r.createNewSecrets(ctx, clusterSecret, ls, namespaceList)
		if err != nil {
			meta.SetStatusCondition(&clusterSecret.Status.Conditions, metav1.Condition{Type: typeAvailableClusterSecret,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Secret: (%s)", err)})
			if err = r.Status().Update(ctx, clusterSecret); err != nil {
				log.Log.Error(err, "Failed to update ClusterSecret status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
	}

	meta.SetStatusCondition(&clusterSecret.Status.Conditions, metav1.Condition{Type: typeAvailableClusterSecret,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: "Secrets creation has finished successfully"})
	if err := r.Status().Update(ctx, clusterSecret); err != nil {
		log.Log.Error(err, "Failed to update ClusterSecret status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ClusterSecretReconciler) createNewSecrets(ctx context.Context, clusterSecret *v12.ClusterSecret, ls map[string]string, namespaceList []string) ([]*v1.Secret, error) {
	var secretList []*v1.Secret
	log.Log.Info("Processing the cluster secret %s from namespace %s", clusterSecret.Name, clusterSecret.Namespace)
	for _, n := range namespaceList {
		log.Log.Info("Checking if the secret %s exists on namespace %s", clusterSecret.Name, n)
		found := &v1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      clusterSecret.Name,
			Namespace: n,
		}, found)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Log.Info("Secret not found, creating a new secret %s on namespace %s", clusterSecret.Name, n)
				newSecret := &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterSecret.Name,
						Namespace: n,
						Labels:    ls,
					},
					Type: clusterSecret.Spec.Type,
					Data: clusterSecret.Spec.Data,
				}
				err = r.Create(ctx, newSecret)
				if err != nil {
					log.Log.Error(err, err.Error())
					return nil, err
				}
				secretList = append(secretList, newSecret)
				continue
			}
			log.Log.Error(err, "Error occurred when querying secret %s on namespace %s", clusterSecret.Name, n)
			return nil, err
		} else {
			log.Log.Info("Secret %s found on namespace %s, updating its values", clusterSecret.Name, n)
			found.Type = clusterSecret.Spec.Type
			found.Data = clusterSecret.Spec.Data
			err = r.Update(ctx, found)
			if err != nil {
				log.Log.Error(err, err.Error())
				return nil, err
			}
			log.Log.Info("Secret %s from namespace %s, updated successfully", clusterSecret.Name, n)
			secretList = append(secretList, found)
		}
	}
	return secretList, nil
}

func (r *ClusterSecretReconciler) filterNamespaceList(k8sNamespaceList *v1.NamespaceList, clusterSecret *v12.ClusterSecret) ([]string, error) {
	// TODO: Improve this filtering logic
	var namespaceList []string
	for _, namespace := range clusterSecret.Spec.IncludeNamespaces {
		if namespace == "*" {
			for _, k8sNamespace := range k8sNamespaceList.Items {
				namespaceList = append(namespaceList, k8sNamespace.Name)
			}
			break
		} else if strings.HasPrefix(namespace, "*") {
			for _, k8sNamespace := range k8sNamespaceList.Items {
				if strings.HasSuffix(k8sNamespace.Name, namespace[1:]) {
					namespaceList = append(namespaceList, k8sNamespace.Name)
				}
			}
		} else if strings.HasSuffix(namespace, "*") {
			for _, k8sNamespace := range k8sNamespaceList.Items {
				if strings.HasSuffix(k8sNamespace.Name, namespace[:1]) {
					namespaceList = append(namespaceList, k8sNamespace.Name)
				}
			}
		} else {
			for _, k8sNamespace := range k8sNamespaceList.Items {
				if k8sNamespace.Name == namespace {
					namespaceList = append(namespaceList, k8sNamespace.Name)
				}
			}
		}
	}
	for _, namespace := range clusterSecret.Spec.ExcludeNamespaces {
		if namespace == "*" {
			return []string{}, nil
		} else if strings.HasPrefix(namespace, "*") {
			for i := 0; i < len(namespaceList); i++ {
				if strings.HasSuffix(namespaceList[i], namespace[1:]) {
					namespaceList[i] = namespaceList[len(namespaceList)-1]
					namespaceList = namespaceList[:len(namespaceList)-1]
					i--
				}
			}
		} else if strings.HasSuffix(namespace, "*") {
			for i := 0; i < len(namespaceList); i++ {
				if strings.HasPrefix(namespaceList[i], namespace[:1]) {
					namespaceList[i] = namespaceList[len(namespaceList)-1]
					namespaceList = namespaceList[:len(namespaceList)-1]
					i--
				}
			}
		} else {
			for i := 0; i < len(namespaceList); i++ {
				if namespaceList[i] == namespace {
					namespaceList[i] = namespaceList[len(namespaceList)-1]
					namespaceList = namespaceList[:len(namespaceList)-1]
					i--
				}
			}
		}
	}
	return namespaceList, nil
}

func getLabels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/cluster-secret": name,
		"app.kubernetes.io/part-of":        "cluster-secret-operator",
		"app.kubernetes.io/created-by":     "controller-manager",
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v12.ClusterSecret{}).
		Complete(r)
}
